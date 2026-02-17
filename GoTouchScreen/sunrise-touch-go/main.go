package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"time"
	"unsafe"

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
	theme         int
	page          int
	showSettings  bool
	showCalibration bool
	calibStep     int
	calibRaw      [3]touchPoint
	settingsLat   float64
	settingsLon   float64
	settingsEvery time.Duration
	manualRedraw  bool
	exitArmedUntil time.Time
	exitRequested bool
}

type touchCalibration struct {
	xScale  float64
	yScale  float64
	xOffset float64
	yOffset float64
}

type persistedConfig struct {
	Lat             float64 `json:"lat"`
	Lon             float64 `json:"lon"`
	IntervalSeconds int64   `json:"interval_seconds"`
	DarkMode        bool    `json:"dark_mode"`
	Theme           int     `json:"theme"`
	CalXScale       float64 `json:"cal_x_scale"`
	CalYScale       float64 `json:"cal_y_scale"`
	CalXOffset      float64 `json:"cal_x_offset"`
	CalYOffset      float64 `json:"cal_y_offset"`
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
	latFlag := flag.Float64("lat", 37.7749, "latitude")
	lonFlag := flag.Float64("lon", -122.4194, "longitude")
	intervalFlag := flag.Duration("interval", 15*time.Minute, "stats refresh interval")
	pollFlag := flag.Duration("poll", 250*time.Millisecond, "touch poll interval")
	partialFlag := flag.Bool("partial", false, "enable partial refresh policy")
	configPath := flag.String("config", "/home/chad/.config/sunrise-touch-go/config.json", "settings file path")
	flag.Parse()

	cfg := persistedConfig{
		Lat:             *latFlag,
		Lon:             *lonFlag,
		IntervalSeconds: int64((*intervalFlag) / time.Second),
		DarkMode:        false,
		Theme:           0,
		CalXScale:       1,
		CalYScale:       1,
		CalXOffset:      0,
		CalYOffset:      0,
	}
	if loaded, err := loadConfig(*configPath); err == nil {
		cfg = loaded
		log.Printf("loaded config: %s", *configPath)
	} else {
		log.Printf("config load skipped: %v", err)
	}
	if cfg.IntervalSeconds < 180 {
		cfg.IntervalSeconds = 180
	}
	if cfg.Theme < 0 || cfg.Theme > 2 {
		if cfg.DarkMode {
			cfg.Theme = 1
		} else {
			cfg.Theme = 0
		}
	}
	lat := cfg.Lat
	lon := cfg.Lon
	refreshEvery := time.Duration(cfg.IntervalSeconds) * time.Second
	if refreshEvery < 180*time.Second {
		refreshEvery = 180 * time.Second
	}
	cal := touchCalibration{
		xScale:  cfg.CalXScale,
		yScale:  cfg.CalYScale,
		xOffset: cfg.CalXOffset,
		yOffset: cfg.CalYOffset,
	}
	if cal.xScale == 0 {
		cal.xScale = 1
	}
	if cal.yScale == 0 {
		cal.yScale = 1
	}

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
	partialEnabled := *partialFlag
	log.Printf("partial policy enabled=%v", partialEnabled)
	_ = setDisplayMode(display, false)
	if err := display.Clear(color.White); err != nil {
		log.Fatal(err)
	}
	displaySleeping := false

	state := appState{theme: cfg.Theme}
	lastTouch := touchPoint{-1, -1}
	lastTouchAt := time.Now().Add(-time.Hour)
	lastTouchErrAt := time.Now().Add(-time.Hour)
	touchHeld := false
	lastDrawAt := time.Time{}
	var lastPortrait *image.Gray
	drawCount := 0
	touchCount := 0
	startedAt := time.Now()
	partialSinceFull := 0
	lastFullRefresh := time.Now()

	for {
		now := time.Now()
		if !state.exitArmedUntil.IsZero() && now.After(state.exitArmedUntil) {
			state.exitArmedUntil = time.Time{}
			state.manualRedraw = true
		}
		sunrise, err := nextSunrise(now, lat, lon, now.Location())
		if err != nil {
			log.Printf("sunrise calc: %v", err)
			sunrise = now.Add(6 * time.Hour)
		}
		until := sunrise.Sub(now)
		shouldDraw := state.manualRedraw || lastDrawAt.IsZero()
		if now.Sub(lastDrawAt) >= refreshEvery {
			shouldDraw = true
		}

		if touch != nil {
			if tp, err := touch.poll(); err != nil {
				if time.Since(lastTouchErrAt) > 3*time.Second {
					log.Printf("touch poll error: %v", err)
					lastTouchErrAt = time.Now()
				}
			} else if tp == nil {
				touchHeld = false
			} else if tp != nil {
				if !touchHeld {
					touchHeld = true
					if tp.x == lastTouch.x && tp.y == lastTouch.y && time.Since(lastTouchAt) < 700*time.Millisecond {
						// Debounce.
					} else {
						lastTouch = *tp
						lastTouchAt = time.Now()
						touchCount++
						rawLX, rawLY := mapTouchToLandscape(tp.x, tp.y)
						lx, ly := applyCalibration(rawLX, rawLY, cal)
						log.Printf("touch: raw=(%d,%d) base=(%d,%d) mapped=(%d,%d)", tp.x, tp.y, rawLX, rawLY, lx, ly)
						handleTouch(&state, rawLX, rawLY, lx, ly, &lat, &lon, &refreshEvery, &cal, *configPath)
						shouldDraw = true
					}
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
					time.Sleep(*pollFlag)
					continue
				}
				_ = setDisplayMode(display, false)
				displaySleeping = false
			}

			drawCount++
			frame := renderLandscape(now, sunrise, until, lat, lon, refreshEvery, drawCount, touchCount, startedAt, partialEnabled, state)
			portrait := landscapeToPortrait(frame)
			img := image1bit.NewVerticalLSB(display.Bounds())
			draw.Draw(img, img.Bounds(), portrait, image.Point{}, draw.Src)
			drawRect := display.Bounds()
			shouldSend := true
			usePartial := partialEnabled && lastPortrait != nil
			forceFull := !usePartial || partialSinceFull >= 6 || now.Sub(lastFullRefresh) >= 24*time.Hour
			if usePartial {
				if diff, ok := diffRectGray(lastPortrait, portrait); ok {
					drawRect = alignRectForEPD(diff, display.Bounds())
				} else {
					shouldSend = false
				}
			}
			if shouldSend {
				if forceFull {
					drawRect = display.Bounds()
					_ = setDisplayMode(display, false)
				} else {
					_ = setDisplayMode(display, true)
				}
				if err := display.Draw(drawRect, img, image.Point{}); err != nil {
					log.Printf("draw failed: %v", err)
				} else if err := display.Sleep(); err != nil {
					log.Printf("display sleep failed: %v", err)
				} else {
					displaySleeping = true
					if forceFull {
						partialSinceFull = 0
						lastFullRefresh = now
					} else {
						partialSinceFull++
					}
				}
			} else if !displaySleeping {
				if err := display.Sleep(); err != nil {
					log.Printf("display sleep failed: %v", err)
				} else {
					displaySleeping = true
				}
			}
			lastPortrait = portrait
			lastDrawAt = now
			state.manualRedraw = false
		}
		time.Sleep(*pollFlag)
	}
}

func mapTouchToLandscape(px, py int) (int, int) {
	// Touch is reported in portrait (122x250). Dashboard is landscape (250x122).
	// This mapping matches a 90-degree clockwise rotation.
	return 249 - py, px
}

func handleTouch(st *appState, rawX, rawY, x, y int, lat, lon *float64, refreshEvery *time.Duration, cal *touchCalibration, configPath string) {
	if st.showCalibration {
		handleCalibrationTouch(st, rawX, rawY, lat, lon, refreshEvery, cal, configPath)
		return
	}

	if st.showSettings {
		handleSettingsTouch(st, x, y, lat, lon, refreshEvery, cal, configPath)
		return
	}

	// Keep touch targets close to visible header buttons to avoid accidental hits.
	buttonTheme := rect{2, 0, 62, 28}
	buttonPage := rect{63, 0, 124, 28}
	buttonSet := rect{125, 0, 186, 28}
	buttonExit := rect{187, 0, 249, 28}
	switch {
	case inside(buttonTheme, x, y):
		st.theme = (st.theme + 1) % 3
		st.manualRedraw = true
		log.Printf("button: THEME %d", st.theme)
	case inside(buttonPage, x, y):
		st.page = (st.page + 1) % 3
		st.manualRedraw = true
		log.Printf("button: PAGE %d", st.page)
	case inside(buttonSet, x, y):
		st.showSettings = true
		st.settingsLat = *lat
		st.settingsLon = *lon
		st.settingsEvery = *refreshEvery
		st.manualRedraw = true
		log.Printf("button: SET")
	case inside(buttonExit, x, y):
		handleExitTap(st)
		log.Printf("button: EXIT tap")
	default:
		log.Printf("button: none")
	}
}

func handleSettingsTouch(st *appState, x, y int, lat, lon *float64, refreshEvery *time.Duration, cal *touchCalibration, configPath string) {
	buttonBack := rect{2, 0, 82, 28}
	buttonSave := rect{83, 0, 165, 28}
	buttonExit := rect{166, 0, 249, 28}
	buttonCal := rect{132, 34, 246, 54}
	latMinus := rect{6, 34, 34, 54}
	latPlus := rect{88, 34, 118, 54}
	lonMinus := rect{6, 62, 34, 82}
	lonPlus := rect{88, 62, 118, 82}
	intMinus := rect{6, 90, 34, 110}
	intPlus := rect{88, 90, 118, 110}
	themeToggle := rect{132, 62, 246, 82}

	switch {
	case inside(buttonBack, x, y):
		st.showSettings = false
		st.manualRedraw = true
		log.Printf("settings: BACK")
	case inside(buttonSave, x, y):
		*lat = st.settingsLat
		*lon = st.settingsLon
		*refreshEvery = st.settingsEvery
		if err := persistRuntimeConfig(configPath, *lat, *lon, *refreshEvery, st.theme, *cal); err != nil {
			log.Printf("settings: save failed: %v", err)
		} else {
			log.Printf("settings: saved to %s", configPath)
		}
		st.showSettings = false
		st.manualRedraw = true
	case inside(buttonCal, x, y):
		st.showCalibration = true
		st.calibStep = 0
		st.manualRedraw = true
		log.Printf("settings: CALIB")
	case inside(buttonExit, x, y):
		handleExitTap(st)
		log.Printf("settings: EXIT tap")
	case inside(latMinus, x, y):
		st.settingsLat -= 0.01
		st.manualRedraw = true
	case inside(latPlus, x, y):
		st.settingsLat += 0.01
		st.manualRedraw = true
	case inside(lonMinus, x, y):
		st.settingsLon -= 0.01
		st.manualRedraw = true
	case inside(lonPlus, x, y):
		st.settingsLon += 0.01
		st.manualRedraw = true
	case inside(intMinus, x, y):
		if st.settingsEvery > 5*time.Minute {
			st.settingsEvery -= 5 * time.Minute
			st.manualRedraw = true
		}
	case inside(intPlus, x, y):
		if st.settingsEvery < 6*time.Hour {
			st.settingsEvery += 5 * time.Minute
			st.manualRedraw = true
		}
	case inside(themeToggle, x, y):
		st.theme = (st.theme + 1) % 3
		st.manualRedraw = true
	default:
		log.Printf("settings: none")
	}
}

func handleCalibrationTouch(st *appState, rawX, rawY int, lat, lon *float64, refreshEvery *time.Duration, cal *touchCalibration, configPath string) {
	buttonBack := rect{2, 0, 120, 28}
	buttonApply := rect{121, 0, 249, 28}
	if inside(buttonBack, rawX, rawY) {
		st.showCalibration = false
		st.manualRedraw = true
		log.Printf("calib: BACK")
		return
	}
	if inside(buttonApply, rawX, rawY) && st.calibStep >= 3 {
		newCal, err := computeCalibration(st.calibRaw)
		if err != nil {
			log.Printf("calib: compute failed: %v", err)
			return
		}
		*cal = newCal
		if err := persistRuntimeConfig(configPath, *lat, *lon, *refreshEvery, st.theme, *cal); err != nil {
			log.Printf("calib: save failed: %v", err)
		} else {
			log.Printf("calib: saved")
		}
		st.showCalibration = false
		st.manualRedraw = true
		return
	}
	if st.calibStep < 3 {
		st.calibRaw[st.calibStep] = touchPoint{x: rawX, y: rawY}
		st.calibStep++
		st.manualRedraw = true
		log.Printf("calib: captured step %d raw=(%d,%d)", st.calibStep, rawX, rawY)
	}
}

func handleExitTap(st *appState) {
	st.exitRequested = true
}

func inside(r rect, x, y int) bool {
	return x >= r.x0 && x <= r.x1 && y >= r.y0 && y <= r.y1
}

func renderLandscape(now, sunrise time.Time, until time.Duration, lat, lon float64, refreshEvery time.Duration, tick, touchCount int, startedAt time.Time, partialEnabled bool, st appState) *image.Gray {
	const w = 250
	const h = 122
	bg, fg := themeColors(st.theme)

	img := image.NewGray(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.Gray{Y: bg}}, image.Point{}, draw.Src)

	if st.showCalibration {
		renderCalibrationView(img, st, fg, bg)
		return img
	}

	if st.showSettings {
		renderSettingsView(img, st, fg, bg)
		return img
	}

	// Header + controls
	line(img, 0, 22, w-1, 22, fg)
	drawButton(img, rect{4, 2, 60, 20}, "THEME", false, fg, bg)
	drawButton(img, rect{64, 2, 120, 20}, "PAGE", false, fg, bg)
	drawButton(img, rect{124, 2, 180, 20}, "SET", false, fg, bg)
	exitLabel := "EXIT"
	if !st.exitArmedUntil.IsZero() && time.Now().Before(st.exitArmedUntil) {
		exitLabel = "EXIT!"
	}
	drawButton(img, rect{184, 2, 246, 20}, exitLabel, !st.exitArmedUntil.IsZero(), fg, bg)

	// Main split
	line(img, 125, 23, 125, h-1, fg)
	text(img, 132, 36, "Sunrise Touch", fg)

	switch st.page {
	case 1:
		renderStatsPage(img, now, refreshEvery, touchCount, tick, startedAt, partialEnabled, fg, bg)
	case 2:
		renderArtPage(img, now, tick, fg, bg)
	default:
		renderSunrisePage(img, now, sunrise, until, lat, lon, refreshEvery, tick, fg)
	}

	return img
}

func renderSettingsView(img *image.Gray, st appState, fg, bg uint8) {
	line(img, 0, 22, 249, 22, fg)
	drawButton(img, rect{4, 2, 80, 20}, "BACK", false, fg, bg)
	drawButton(img, rect{84, 2, 160, 20}, "SAVE", false, fg, bg)
	exitLabel := "EXIT"
	if !st.exitArmedUntil.IsZero() && time.Now().Before(st.exitArmedUntil) {
		exitLabel = "EXIT!"
	}
	drawButton(img, rect{164, 2, 246, 20}, exitLabel, !st.exitArmedUntil.IsZero(), fg, bg)

	text(img, 8, 36, "Settings", fg)
	text(img, 8, 50, "LAT", fg)
	text(img, 8, 78, "LON", fg)
	text(img, 8, 106, "REFRESH", fg)

	text(img, 40, 50, fmt.Sprintf("%.3f", st.settingsLat), fg)
	text(img, 40, 78, fmt.Sprintf("%.3f", st.settingsLon), fg)
	text(img, 58, 106, fmt.Sprintf("%dm", int(st.settingsEvery.Minutes())), fg)
	text(img, 132, 50, "-/+", fg)
	text(img, 132, 78, "-/+", fg)
	text(img, 132, 106, "-/+", fg)

	drawButton(img, rect{6, 34, 34, 54}, "-", false, fg, bg)
	drawButton(img, rect{88, 34, 118, 54}, "+", false, fg, bg)
	drawButton(img, rect{6, 62, 34, 82}, "-", false, fg, bg)
	drawButton(img, rect{88, 62, 118, 82}, "+", false, fg, bg)
	drawButton(img, rect{6, 90, 34, 110}, "-", false, fg, bg)
	drawButton(img, rect{88, 90, 118, 110}, "+", false, fg, bg)

	drawButton(img, rect{132, 62, 246, 82}, "THEME CYCLE", false, fg, bg)
	drawButton(img, rect{132, 34, 246, 54}, "CALIBRATE", false, fg, bg)
	text(img, 132, 98, fmt.Sprintf("theme:%d", st.theme), fg)
	text(img, 132, 112, "persist on SAVE", fg)
}

func renderCalibrationView(img *image.Gray, st appState, fg, bg uint8) {
	line(img, 0, 22, 249, 22, fg)
	drawButton(img, rect{4, 2, 116, 20}, "BACK", false, fg, bg)
	applyActive := st.calibStep >= 3
	drawButton(img, rect{120, 2, 246, 20}, "APPLY", applyActive, fg, bg)
	text(img, 8, 36, "Touch 3 targets", fg)

	targets := [3]touchPoint{
		{x: 20, y: 38},
		{x: 230, y: 38},
		{x: 125, y: 106},
	}
	for i := 0; i < 3; i++ {
		cx, cy := targets[i].x, targets[i].y
		done := i < st.calibStep
		if done {
			circle(img, cx, cy, 8, fg, true)
		} else {
			circle(img, cx, cy, 8, fg, false)
		}
		line(img, cx-10, cy, cx+10, cy, fg)
		line(img, cx, cy-10, cx, cy+10, fg)
	}
	stepText := "step 1/3"
	if st.calibStep == 1 {
		stepText = "step 2/3"
	} else if st.calibStep == 2 {
		stepText = "step 3/3"
	} else if st.calibStep >= 3 {
		stepText = "ready: tap APPLY"
	}
	text(img, 8, 118, stepText, fg)
}

func themeColors(theme int) (uint8, uint8) {
	switch theme {
	case 1:
		return 0, 255
	case 2:
		return 255, 0
	default:
		return 255, 0
	}
}

func renderSunrisePage(img *image.Gray, now, sunrise time.Time, until time.Duration, lat, lon float64, refreshEvery time.Duration, tick int, fg uint8) {
	untilStr := formatDur(until)
	text(img, 8, 38, "NEXT SUNRISE", fg)
	text(img, 8, 58, untilStr, fg)
	text(img, 8, 76, sunrise.Format("03:04:05 PM"), fg)
	text(img, 8, 94, fmt.Sprintf("LAT %.4f", lat), fg)
	text(img, 8, 110, fmt.Sprintf("LON %.4f R:%dm", lon, int(refreshEvery.Minutes())), fg)

	line(img, 130, 90, 245, 90, fg)
	secOfDay := now.Hour()*3600 + now.Minute()*60 + now.Second()
	p := float64(secOfDay) / 86400.0
	sunX := 132 + int(110*p)
	sunY := 90 - int(26*math.Sin((p-0.25)*2*math.Pi))
	circle(img, sunX, sunY, 8, fg, false)
	cloudX := 132 + (tick*7)%108
	line(img, cloudX, 42, cloudX+16, 42, fg)
	line(img, cloudX+2, 39, cloudX+14, 39, fg)
	text(img, 132, 108, now.Format("Mon 03:04 PM"), fg)
}

func renderStatsPage(img *image.Gray, now time.Time, refreshEvery time.Duration, touchCount, drawCount int, startedAt time.Time, partialEnabled bool, fg, bg uint8) {
	text(img, 8, 38, "SYSTEM STATS", fg)
	text(img, 8, 56, "UPTIME "+formatDur(time.Since(startedAt)), fg)
	text(img, 8, 74, fmt.Sprintf("DRAWS %d", drawCount), fg)
	text(img, 8, 92, fmt.Sprintf("TOUCH %d", touchCount), fg)
	text(img, 8, 110, fmt.Sprintf("RFR %dm", int(refreshEvery.Minutes())), fg)

	mode := "FULL"
	if partialEnabled {
		mode = "PARTIAL"
	}
	text(img, 132, 38, "DISPLAY MODE", fg)
	drawButton(img, rect{132, 46, 246, 66}, mode, partialEnabled, fg, bg)
	text(img, 132, 86, "PAGE: STATS", fg)
	text(img, 132, 104, now.Format("Jan 02 03:04"), fg)
}

func renderArtPage(img *image.Gray, now time.Time, tick int, fg, bg uint8) {
	text(img, 8, 38, "MONO ART", fg)
	for i := 0; i < 6; i++ {
		x := 10 + i*18 + (tick % 6)
		circle(img, x, 72, 7+i%3, fg, false)
	}
	for y := 42; y <= 110; y += 8 {
		line(img, 130, y, 246, y-18+(tick%12), fg)
	}
	if bg == 255 {
		for y := 24; y < 122; y += 4 {
			img.SetGray(126+(y%5), y, color.Gray{Y: fg})
		}
	}
	text(img, 132, 108, now.Format("03:04 PM"), fg)
}

func loadConfig(path string) (persistedConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return persistedConfig{}, err
	}
	var cfg persistedConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return persistedConfig{}, err
	}
	return cfg, nil
}

func saveConfig(path string, cfg persistedConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func persistRuntimeConfig(path string, lat, lon float64, refreshEvery time.Duration, theme int, cal touchCalibration) error {
	cfg := persistedConfig{
		Lat:             lat,
		Lon:             lon,
		IntervalSeconds: int64(refreshEvery / time.Second),
		DarkMode:        theme == 1,
		Theme:           theme,
		CalXScale:       cal.xScale,
		CalYScale:       cal.yScale,
		CalXOffset:      cal.xOffset,
		CalYOffset:      cal.yOffset,
	}
	return saveConfig(path, cfg)
}

func applyCalibration(x, y int, cal touchCalibration) (int, int) {
	cx := int(math.Round(float64(x)*cal.xScale + cal.xOffset))
	cy := int(math.Round(float64(y)*cal.yScale + cal.yOffset))
	if cx < 0 {
		cx = 0
	}
	if cx > 249 {
		cx = 249
	}
	if cy < 0 {
		cy = 0
	}
	if cy > 121 {
		cy = 121
	}
	return cx, cy
}

func computeCalibration(raw [3]touchPoint) (touchCalibration, error) {
	targets := [3]touchPoint{
		{x: 20, y: 38},
		{x: 230, y: 38},
		{x: 125, y: 106},
	}
	dx := float64(raw[1].x - raw[0].x)
	dy := float64(raw[2].y - raw[0].y)
	if math.Abs(dx) < 2 || math.Abs(dy) < 2 {
		return touchCalibration{}, errors.New("touch points too close, retry calibration")
	}
	xScale := float64(targets[1].x-targets[0].x) / dx
	yScale := float64(targets[2].y-targets[0].y) / dy
	xOffset := float64(targets[0].x) - float64(raw[0].x)*xScale
	yOffset := float64(targets[0].y) - float64(raw[0].y)*yScale
	if math.Abs(xScale) > 3 || math.Abs(yScale) > 3 {
		return touchCalibration{}, errors.New("invalid scale computed, retry calibration")
	}
	return touchCalibration{xScale: xScale, yScale: yScale, xOffset: xOffset, yOffset: yOffset}, nil
}

func setDisplayMode(display *waveshare2in13v4.Dev, partial bool) error {
	v := reflect.ValueOf(display).Elem().FieldByName("mode")
	if !v.IsValid() || !v.CanAddr() {
		return errors.New("display mode field unavailable")
	}
	ptr := reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
	if partial {
		ptr.Set(reflect.ValueOf(waveshare2in13v4.Partial))
	} else {
		ptr.Set(reflect.ValueOf(waveshare2in13v4.Full))
	}
	return nil
}

func alignRectForEPD(r, bounds image.Rectangle) image.Rectangle {
	if r.Empty() {
		return r
	}
	x0 := r.Min.X &^ 7
	x1 := (r.Max.X + 7) &^ 7
	if x0 < bounds.Min.X {
		x0 = bounds.Min.X
	}
	if x1 > bounds.Max.X {
		x1 = bounds.Max.X
	}
	if x1 <= x0 {
		return bounds
	}
	return image.Rect(x0, r.Min.Y, x1, r.Max.Y).Intersect(bounds)
}

func diffRectGray(prev, curr *image.Gray) (image.Rectangle, bool) {
	if prev == nil || !prev.Rect.Eq(curr.Rect) {
		return curr.Bounds(), true
	}
	minX, minY := curr.Rect.Max.X, curr.Rect.Max.Y
	maxX, maxY := curr.Rect.Min.X, curr.Rect.Min.Y
	changed := false
	for y := curr.Rect.Min.Y; y < curr.Rect.Max.Y; y++ {
		for x := curr.Rect.Min.X; x < curr.Rect.Max.X; x++ {
			if prev.GrayAt(x, y) != curr.GrayAt(x, y) {
				changed = true
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if !changed {
		return image.Rectangle{}, false
	}
	return image.Rect(minX, minY, maxX+1, maxY+1), true
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
