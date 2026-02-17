package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"math"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"periph.io/x/conn/v3/i2c"
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/conn/v3/spi/spireg"
	"periph.io/x/devices/v3/ssd1306/image1bit"
	"periph.io/x/devices/v3/waveshare2in13v4"
	"periph.io/x/host/v3"
)

type rect struct {
	x0 int
	y0 int
	x1 int
	y1 int
}

type appState struct {
	invert       bool
	manualRedraw bool
	exitRequested bool
}

type gt1151 struct {
	bus i2c.BusCloser
	dev *i2c.Dev
}

type touchPoint struct {
	x int
	y int
}

func newGT1151() (*gt1151, error) {
	bus, err := i2creg.Open("1")
	if err != nil {
		return nil, err
	}
	return &gt1151{
		bus: bus,
		dev: &i2c.Dev{Bus: bus, Addr: 0x14},
	}, nil
}

func (g *gt1151) Close() error {
	if g.bus != nil {
		return g.bus.Close()
	}
	return nil
}

func (g *gt1151) read(reg uint16, n int) ([]byte, error) {
	w := []byte{byte(reg >> 8), byte(reg & 0xFF)}
	r := make([]byte, n)
	if err := g.dev.Tx(w, r); err != nil {
		return nil, err
	}
	return r, nil
}

func (g *gt1151) write(reg uint16, b byte) error {
	w := []byte{byte(reg >> 8), byte(reg & 0xFF), b}
	return g.dev.Tx(w, nil)
}

func (g *gt1151) poll() (*touchPoint, error) {
	status, err := g.read(0x814E, 1)
	if err != nil {
		return nil, err
	}
	if status[0]&0x80 == 0 {
		return nil, nil
	}
	count := int(status[0] & 0x0F)
	if count < 1 || count > 5 {
		_ = g.write(0x814E, 0x00)
		return nil, nil
	}
	data, err := g.read(0x814F, count*8)
	if err != nil {
		return nil, err
	}
	_ = g.write(0x814E, 0x00)
	x := int(data[1]) | int(data[2])<<8
	y := int(data[3]) | int(data[4])<<8
	if x < 0 || x > 121 || y < 0 || y > 249 {
		return nil, nil
	}
	return &touchPoint{x: x, y: y}, nil
}

func main() {
	lat := flag.Float64("lat", 37.7749, "latitude")
	lon := flag.Float64("lon", -122.4194, "longitude")
	interval := flag.Duration("interval", 15*time.Minute, "stats refresh interval")
	poll := flag.Duration("poll", 250*time.Millisecond, "touch poll interval")
	flag.Parse()

	if _, err := host.Init(); err != nil {
		log.Fatal(err)
	}

	spiPort, err := spireg.Open("")
	if err != nil {
		log.Fatal(err)
	}
	defer spiPort.Close()

	opts := waveshare2in13v4.EPD2in13v4
	display, err := waveshare2in13v4.NewHat(spiPort, &opts)
	if err != nil {
		log.Fatal(err)
	}
	defer display.Halt()

	touch, err := newGT1151()
	if err != nil {
		log.Printf("touch unavailable: %v", err)
	} else {
		defer touch.Close()
	}

	if err := display.Init(); err != nil {
		log.Fatal(err)
	}
	if err := display.Clear(color.White); err != nil {
		log.Fatal(err)
	}
	displaySleeping := false

	state := appState{}
	lastTouch := touchPoint{-1, -1}
	lastTouchAt := time.Now().Add(-time.Hour)
	lastTouchErrAt := time.Now().Add(-time.Hour)
	lastDrawAt := time.Time{}
	drawCount := 0

	for {
		now := time.Now()
		sunrise, err := nextSunrise(now, *lat, *lon, now.Location())
		if err != nil {
			log.Printf("sunrise calc: %v", err)
			sunrise = now.Add(6 * time.Hour)
		}
		until := sunrise.Sub(now)
		shouldDraw := state.manualRedraw || lastDrawAt.IsZero()
		if now.Sub(lastDrawAt) >= *interval {
			shouldDraw = true
		}

		if touch != nil {
			if tp, err := touch.poll(); err != nil {
				if time.Since(lastTouchErrAt) > 3*time.Second {
					log.Printf("touch poll error: %v", err)
					lastTouchErrAt = time.Now()
				}
			} else if tp != nil {
				if tp.x == lastTouch.x && tp.y == lastTouch.y && time.Since(lastTouchAt) < 700*time.Millisecond {
					// Debounce.
				} else {
					lastTouch = *tp
					lastTouchAt = time.Now()
					lx, ly := mapTouchToLandscape(tp.x, tp.y)
					log.Printf("touch: raw=(%d,%d) mapped=(%d,%d)", tp.x, tp.y, lx, ly)
					handleTouch(&state, lx, ly)
					shouldDraw = true
				}
			}
		}

		if state.exitRequested {
			if displaySleeping {
				if err := display.Init(); err != nil {
					log.Printf("display wake/init for exit failed: %v", err)
				}
				displaySleeping = false
			}
			if err := display.Clear(color.White); err != nil {
				log.Printf("exit clear failed: %v", err)
			}
			if err := display.Sleep(); err != nil {
				log.Printf("exit sleep failed: %v", err)
			}
			log.Printf("exit requested, shutting down app")
			return
		}

		if shouldDraw {
			if displaySleeping {
				if err := display.Init(); err != nil {
					log.Printf("display wake/init failed: %v", err)
					time.Sleep(*poll)
					continue
				}
				displaySleeping = false
			}

			drawCount++
			frame := renderLandscape(now, sunrise, until, *lat, *lon, drawCount, state)
			portrait := landscapeToPortrait(frame)
			img := image1bit.NewVerticalLSB(display.Bounds())
			draw.Draw(img, img.Bounds(), portrait, image.Point{}, draw.Src)
			if err := display.Draw(display.Bounds(), img, image.Point{}); err != nil {
				log.Printf("draw failed: %v", err)
			} else if err := display.Sleep(); err != nil {
				log.Printf("display sleep failed: %v", err)
			} else {
				displaySleeping = true
			}
			lastDrawAt = now
			state.manualRedraw = false
		}
		time.Sleep(*poll)
	}
}

func mapTouchToLandscape(px, py int) (int, int) {
	// Touch is reported in portrait (122x250). Dashboard is landscape (250x122).
	// This mapping matches a 90-degree clockwise rotation.
	return 249 - py, px
}

func handleTouch(st *appState, x, y int) {
	// Use broad top-row zones so touches remain reliable despite panel variance.
	buttonLight := rect{0, 0, 82, 44}
	buttonDark := rect{83, 0, 165, 44}
	buttonExit := rect{166, 0, 249, 44}
	switch {
	case inside(buttonLight, x, y):
		st.invert = false
		st.manualRedraw = true
		log.Printf("button: LIGHT")
	case inside(buttonDark, x, y):
		st.invert = true
		st.manualRedraw = true
		log.Printf("button: DARK")
	case inside(buttonExit, x, y):
		st.exitRequested = true
		log.Printf("button: EXIT")
	default:
		log.Printf("button: none")
	}
}

func inside(r rect, x, y int) bool {
	return x >= r.x0 && x <= r.x1 && y >= r.y0 && y <= r.y1
}

func renderLandscape(now, sunrise time.Time, until time.Duration, lat, lon float64, tick int, st appState) *image.Gray {
	const w = 250
	const h = 122
	bg := uint8(255)
	fg := uint8(0)
	if st.invert {
		bg, fg = fg, bg
	}

	img := image.NewGray(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.Gray{Y: bg}}, image.Point{}, draw.Src)

	// Header + controls
	line(img, 0, 22, w-1, 22, fg)
	drawButton(img, rect{4, 2, 80, 20}, "LIGHT", !st.invert, fg, bg)
	drawButton(img, rect{84, 2, 160, 20}, "DARK", st.invert, fg, bg)
	drawButton(img, rect{164, 2, 246, 20}, "EXIT", false, fg, bg)

	// Main split
	line(img, 125, 23, 125, h-1, fg)
	text(img, 132, 36, "Sunrise Touch", fg)

	// Left panel: countdown
	untilStr := formatDur(until)
	text(img, 8, 38, "NEXT SUNRISE", fg)
	text(img, 8, 58, untilStr, fg)
	text(img, 8, 76, sunrise.Format("03:04:05 PM"), fg)
	text(img, 8, 94, fmt.Sprintf("LAT %.4f", lat), fg)
	text(img, 8, 110, fmt.Sprintf("LON %.4f", lon), fg)

	// Right panel: simple animation scene.
	// Horizon
	line(img, 130, 90, 245, 90, fg)
	// Sun path and motion
	secOfDay := now.Hour()*3600 + now.Minute()*60 + now.Second()
	p := float64(secOfDay) / 86400.0
	sunX := 132 + int(110*p)
	sunY := 90 - int(26*math.Sin((p-0.25)*2*math.Pi))
	circle(img, sunX, sunY, 8, fg, false)
	cloudX := 132 + (tick*7)%108
	line(img, cloudX, 42, cloudX+16, 42, fg)
	line(img, cloudX+2, 39, cloudX+14, 39, fg)
	text(img, 132, 108, now.Format("Mon 03:04 PM"), fg)

	return img
}

func drawButton(img *image.Gray, r rect, label string, active bool, fg, bg uint8) {
	fill := bg
	ink := fg
	if active {
		fill = fg
		ink = bg
	}
	fillRect(img, r.x0, r.y0, r.x1, r.y1, fill)
	rectOutline(img, r.x0, r.y0, r.x1, r.y1, fg)
	text(img, r.x0+4, r.y0+14, label, ink)
}

func text(img *image.Gray, x, y int, s string, fg uint8) {
	d := font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.Gray{Y: fg}),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(s)
}

func fillRect(img *image.Gray, x0, y0, x1, y1 int, c uint8) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if image.Pt(x, y).In(img.Rect) {
				img.SetGray(x, y, color.Gray{Y: c})
			}
		}
	}
}

func rectOutline(img *image.Gray, x0, y0, x1, y1 int, c uint8) {
	line(img, x0, y0, x1, y0, c)
	line(img, x0, y1, x1, y1, c)
	line(img, x0, y0, x0, y1, c)
	line(img, x1, y0, x1, y1, c)
}

func line(img *image.Gray, x0, y0, x1, y1 int, c uint8) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		if image.Pt(x0, y0).In(img.Rect) {
			img.SetGray(x0, y0, color.Gray{Y: c})
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func circle(img *image.Gray, cx, cy, r int, c uint8, fill bool) {
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			d := x*x + y*y
			if fill {
				if d <= r*r {
					px, py := cx+x, cy+y
					if image.Pt(px, py).In(img.Rect) {
						img.SetGray(px, py, color.Gray{Y: c})
					}
				}
			} else if d >= (r-1)*(r-1) && d <= r*r {
				px, py := cx+x, cy+y
				if image.Pt(px, py).In(img.Rect) {
					img.SetGray(px, py, color.Gray{Y: c})
				}
			}
		}
	}
}

func landscapeToPortrait(src *image.Gray) *image.Gray {
	// landscape 250x122 -> portrait 122x250
	dst := image.NewGray(image.Rect(0, 0, 122, 250))
	for y := 0; y < 250; y++ {
		for x := 0; x < 122; x++ {
			sx := y
			sy := 121 - x
			dst.SetGray(x, y, src.GrayAt(sx, sy))
		}
	}
	return dst
}

func formatDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func nextSunrise(now time.Time, lat, lon float64, loc *time.Location) (time.Time, error) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	sr, err := sunriseForDate(day, lat, lon, loc)
	if err != nil {
		return time.Time{}, err
	}
	if now.Before(sr) {
		return sr, nil
	}
	return sunriseForDate(day.Add(24*time.Hour), lat, lon, loc)
}

func sunriseForDate(day time.Time, lat, lon float64, loc *time.Location) (time.Time, error) {
	// NOAA-style approximation.
	n := float64(day.YearDay())
	lngHour := lon / 15.0
	t := n + (6.0-lngHour)/24.0
	m := 0.9856*t - 3.289
	l := normalizeDeg(m + 1.916*math.Sin(deg2rad(m)) + 0.020*math.Sin(2*deg2rad(m)) + 282.634)
	ra := normalizeDeg(rad2deg(math.Atan(0.91764 * math.Tan(deg2rad(l)))))
	lQuadrant := math.Floor(l/90.0) * 90.0
	raQuadrant := math.Floor(ra/90.0) * 90.0
	ra = (ra + (lQuadrant - raQuadrant)) / 15.0
	sinDec := 0.39782 * math.Sin(deg2rad(l))
	cosDec := math.Cos(math.Asin(sinDec))
	cosH := (math.Cos(deg2rad(90.833)) - sinDec*math.Sin(deg2rad(lat))) / (cosDec * math.Cos(deg2rad(lat)))
	if cosH > 1 || cosH < -1 {
		return time.Time{}, errors.New("sunrise unavailable for this date/lat")
	}
	h := (360.0 - rad2deg(math.Acos(cosH))) / 15.0
	localT := h + ra - 0.06571*t - 6.622
	ut := normalizeHour(localT - lngHour)
	utc := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC).
		Add(time.Duration(ut * float64(time.Hour)))
	return utc.In(loc), nil
}

func deg2rad(v float64) float64 { return v * math.Pi / 180.0 }
func rad2deg(v float64) float64 { return v * 180.0 / math.Pi }

func normalizeDeg(v float64) float64 {
	for v < 0 {
		v += 360
	}
	for v >= 360 {
		v -= 360
	}
	return v
}

func normalizeHour(v float64) float64 {
	for v < 0 {
		v += 24
	}
	for v >= 24 {
		v -= 24
	}
	return v
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
