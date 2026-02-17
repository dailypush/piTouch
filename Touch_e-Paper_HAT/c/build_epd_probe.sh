#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

TARGET="${1:-v2}"
mkdir -p bin

COMMON_CFLAGS=(
  -g
  -O2
  -Wall
  -D USE_DEV_LIB
  -I ./lib/Config
  -I ./lib/EPD
)

COMMON_SOURCES=(
  ./lib/Config/DEV_Config.c
  ./lib/Config/dev_hardware_SPI.c
  ./lib/Config/dev_hardware_i2c.c
  ./lib/Config/RPI_sysfs_gpio.c
)

if [[ "$TARGET" == "v2" ]]; then
  gcc "${COMMON_CFLAGS[@]}" \
    ./examples/EPD_2in13_probe_v2.c \
    ./lib/EPD/EPD_2in13_V2.c \
    "${COMMON_SOURCES[@]}" \
    -o ./bin/epd_probe_v2 -lm -lpthread
  echo "Built ./bin/epd_probe_v2"
elif [[ "$TARGET" == "v3" ]]; then
  gcc "${COMMON_CFLAGS[@]}" \
    ./examples/EPD_2in13_probe_v3.c \
    ./lib/EPD/EPD_2in13_V3.c \
    "${COMMON_SOURCES[@]}" \
    -o ./bin/epd_probe_v3 -lm -lpthread
  echo "Built ./bin/epd_probe_v3"
else
  echo "Usage: $0 [v2|v3]" >&2
  exit 2
fi
