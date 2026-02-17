#!/usr/bin/env bash
set -euo pipefail

MODE="${1:-v2}"
HOST="${2:-chad@raspberrypi-screen.local}"
REMOTE_ROOT="${3:-~/piTouch}"
LOCAL_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REMOTE_C_DIR="$REMOTE_ROOT/Touch_e-Paper_HAT/c"

if [[ "$MODE" != "v2" && "$MODE" != "v3" ]]; then
  echo "Usage: $0 [v2|v3] [host] [remote_root]" >&2
  exit 2
fi

echo "Checking for stuck D-state display processes..."
DSTATE=$(ssh "$HOST" "ps -eo stat,cmd | awk '/^D/ && /(sysstats_2in13|waveshare_2in13_sample|python3 -)/{print}'") || true
if [[ -n "${DSTATE:-}" ]]; then
  echo "Found D-state process(es) that require reboot before testing:" >&2
  echo "$DSTATE" >&2
  exit 1
fi

echo "Stopping stale display processes on $HOST ..."
ssh "$HOST" "sudo -n pkill -f '[s]ysstats_2in13' >/dev/null 2>&1 || true; sudo -n pkill -f '[w]aveshare_2in13_sample' >/dev/null 2>&1 || true; sudo -n pkill -f '[e]pd_probe_v2' >/dev/null 2>&1 || true; sudo -n pkill -f '[e]pd_probe_v3' >/dev/null 2>&1 || true"

echo "Syncing C sources to $HOST:$REMOTE_C_DIR ..."
ssh "$HOST" "mkdir -p $REMOTE_ROOT/Touch_e-Paper_HAT"
rsync -az --delete "$LOCAL_ROOT/Touch_e-Paper_HAT/c/" "$HOST:$REMOTE_C_DIR/"

echo "Building probe ($MODE)..."
ssh "$HOST" "cd $REMOTE_C_DIR && ./build_epd_probe.sh $MODE"

BIN="epd_probe_$MODE"
echo "Running one-shot probe: $BIN"
ssh -tt "$HOST" "cd $REMOTE_C_DIR && sudo -n ./bin/$BIN"
