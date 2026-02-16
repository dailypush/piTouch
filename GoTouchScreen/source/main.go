package main

import (
	"log"

	"github.com/periph/cmd/epd/epd_2in9_v2"
	"github.com/periph/conn/gpio/gpioreg"
	"github.com/periph/host"
)

func main() {
	// Initialize periph host.
	if _, err := host.Init(); err != nil {
		log.Fatal(err)
	}

	// Initialize the e-paper display.
	resetPin := gpioreg.ByName("22")
	dcPin := gpioreg.ByName("17")
	busyPin := gpioreg.ByName("24")
	epd, err := epd_2in9_v2.New(resetPin, dcPin, busyPin)
	if err != nil {
		log.Fatal(err)
	}
	defer epd.Close()

	// Your e-paper display code goes here.
}
