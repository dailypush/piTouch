#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

mkdir -p bin

CFLAGS=(
  -g
  -O2
  -Wall
  -D USE_DEV_LIB
  -I ./lib/Config
  -I ./lib/Driver
  -I ./lib/EPD
  -I ./lib/GUI
)

SOURCES=(
  ./examples/main_sysstats_2in13.c
  ./examples/SysStats.c
  ./lib/Driver/GT1151.c
  ./lib/GUI/GUI_Paint.c
  ./lib/GUI/GUI_BMPfile.c
  ./lib/EPD/EPD_2in13_V2.c
  ./lib/Config/DEV_Config.c
  ./lib/Config/dev_hardware_SPI.c
  ./lib/Config/dev_hardware_i2c.c
  ./lib/Config/RPI_sysfs_gpio.c
  ./lib/Fonts/font8.c
  ./lib/Fonts/font12.c
  ./lib/Fonts/font16.c
  ./lib/Fonts/font20.c
  ./lib/Fonts/font24.c
)

gcc "${CFLAGS[@]}" "${SOURCES[@]}" -o ./bin/sysstats_2in13 -lm -lpthread

echo "Built: $SCRIPT_DIR/bin/sysstats_2in13"
echo "Run with: sudo $SCRIPT_DIR/bin/sysstats_2in13"
