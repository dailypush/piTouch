#!/usr/bin/env bash
set -euo pipefail

HOST="${1:-chad@raspberrypi-screen.local}"
REMOTE_PY_DIR="${2:-/home/chad/Touch_e-Paper_Code/python}"

echo "Clearing display on $HOST ..."
ssh -tt "$HOST" "cd '$REMOTE_PY_DIR' && sudo -n python3 - <<'PY'
import sys
sys.path.insert(0, 'lib')
from TP_lib import epd2in13_V4

epd = epd2in13_V4.EPD()
epd.init(epd.FULL_UPDATE)
epd.Clear(0xFF)
epd.sleep()
print('display cleared to white')
PY"
