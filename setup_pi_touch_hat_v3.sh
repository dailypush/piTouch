#!/usr/bin/env bash
set -euo pipefail

HOST="${1:-chad@raspberrypi-screen.local}"
REMOTE_HOME="${2:-/home/chad}"
WAVESHARE_URL="https://files.waveshare.com/upload/4/4e/Touch_e-Paper_Code.zip"

run_remote() {
  ssh -tt "$HOST" "$1"
}

echo "[1/7] Updating apt and installing dependencies on $HOST ..."
run_remote "
set -e
sudo -n apt-get update
sudo -n apt-get install -y \
  unzip wget ca-certificates \
  raspi-config i2c-tools \
  python3-pip python3-setuptools python3-pil python3-numpy \
  python3-gpiozero python3-libgpiod python3-rpi-lgpio python3-lgpio \
  gpiod libgpiod-dev liblgpio1
"

echo "[2/7] Enabling SPI and I2C via raspi-config ..."
run_remote "
set -e
sudo -n raspi-config nonint do_spi 0 || true
sudo -n raspi-config nonint do_i2c 0 || true
"

echo "[3/7] Adding user groups (spi/i2c/gpio) ..."
run_remote "
set -e
TARGET_USER=\"$(basename \"$REMOTE_HOME\")\"
sudo -n usermod -aG spi,i2c,gpio \"$TARGET_USER\" || true
"

echo "[4/7] Downloading latest Waveshare Touch_e-Paper_Code ..."
run_remote "
set -e
cd '$REMOTE_HOME'
rm -rf Touch_e-Paper_Code Touch_e-Paper_Code.zip
wget -O Touch_e-Paper_Code.zip '$WAVESHARE_URL'
unzip -q Touch_e-Paper_Code.zip -d Touch_e-Paper_Code
"

echo "[5/7] Installing Waveshare Python package ..."
run_remote "
set -e
cd '$REMOTE_HOME/Touch_e-Paper_Code/python'
sudo -n python3 setup.py install
"

echo "[6/7] Verifying interfaces and touch address ..."
run_remote "
set -e
echo 'SPI enabled: ' && raspi-config nonint get_spi
echo 'I2C enabled: ' && raspi-config nonint get_i2c
ls -l /dev/spidev0.0 /dev/spidev0.1 /dev/i2c-1
sudo -n i2cdetect -y 1 | sed -n '1,5p'
"

echo "[7/7] Smoke test command (not auto-run):"
echo "ssh $HOST \"cd $REMOTE_HOME/Touch_e-Paper_Code/python/examples && sudo python3 TP2in13_V4_test.py\""

echo
echo "Setup complete. Reboot is recommended on a fresh image:"
echo "ssh $HOST \"sudo reboot\""
