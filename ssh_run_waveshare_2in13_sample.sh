#!/usr/bin/env bash
set -euo pipefail

HOST="${1:-chad@raspberrypi-screen.local}"
REMOTE_ROOT="${2:-~/piTouch}"
LOCAL_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REMOTE_C_DIR="$REMOTE_ROOT/Touch_e-Paper_HAT/c"

echo "Stopping stale display processes on $HOST ..."
ssh "$HOST" "sudo -n pkill -x main >/dev/null 2>&1 || true; sudo -n pkill -f '[s]ysstats_2in13' >/dev/null 2>&1 || true"

echo "Syncing C sources to $HOST:$REMOTE_C_DIR ..."
ssh "$HOST" "mkdir -p $REMOTE_ROOT/Touch_e-Paper_HAT"
rsync -az --delete "$LOCAL_ROOT/Touch_e-Paper_HAT/c/" "$HOST:$REMOTE_C_DIR/"

echo "Building Waveshare 2.13 V2 sample ..."
ssh "$HOST" "cd $REMOTE_C_DIR && ./build_waveshare_2in13.sh"

echo "Running Waveshare sample (TestCode_2in13) ..."
ssh -tt "$HOST" "cd $REMOTE_C_DIR && sudo -n ./bin/waveshare_2in13_sample"
