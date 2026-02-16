package main

import (
	"image"
	"image/draw"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/golang/freetype/truetype"
	"github.com/llgcode/draw2d/draw2dimg"
	"periph.io/x/conn/v3/gpio/gpioreg"
	"periph.io/x/devices/v3/ssd1306/image1bit"
	"periph.io/x/devices/v3/waveshare/epd"
	"periph.io/x/host/v3"
)

func main() {
	if _, err := host.Init(); err != nil {
		log.Fatal(err)
	}

	dc := gpioreg.ByName("P1_22")
	rst := gpioreg.ByName("P1_11")
	busy := gpioreg.ByName("P1_18")

	display, err := epd.NewSPI(epd.EPD2in9, dc, rst, busy, nil)
	if err != nil {
		log.Fatal(err)
	}

	display.Init(false)
	display.Clear()

	img := image1bit.NewVerticalLSB(display.Bounds())
	draw.Draw(img, img.Bounds(), &image.Uniform{image.White}, image.ZP, draw.Src)

	drawer := draw2dimg.NewGraphicContext(img)
	drawer.SetStrokeColor(image.Black)
	drawer.SetFillColor(image.Black)

	ft, err := loadFont()
	if err != nil {
		log.Fatal(err)
	}

	drawer.SetFont(&truetype.Font{
		FUnitsPerEm: ft.FUnitsPerEm,
		GlyphBuf:    ft.GlyphBuf,
		HMetric:     ft.HMetric,
		Kerning:     ft.Kerning,
		Loc:         ft.Loc,
		Name:        ft.Name,
		NumGlyphs:   ft.NumGlyphs,
		PostScript:  ft.PostScript,
		UnderPos:    ft.UnderPos,
		UnderThick:  ft.UnderThick,
		VMetric:     ft.VMetric,
		VMetrics:    ft.VMetrics,
		XHeight:     ft.XHeight,
	})

	drawer.SetFontSize(12)
	drawer.DrawString("Hello, World!", 10, 20)
	display.DrawImage(img)

	time.Sleep(5 * time.Second)
	display.Clear()
	display.Halt()
}

func loadFont() (*truetype.Font, error) {
	f, err := os.Open("path/to/your/font.ttf")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fontBytes, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	parsedFont, err := truetype.Parse(fontBytes)
	if err != nil {
		return nil, err
	}

	return parsedFont, nil
}
