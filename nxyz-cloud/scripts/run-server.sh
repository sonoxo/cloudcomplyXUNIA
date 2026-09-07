#!/usr/bin/env bash
set -eo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STATE_DIR="${NXYZ_LOCAL_STATE_DIR:-$ROOT/.nxyz}"
export NXYZ_LOCAL_STATE_DIR="$STATE_DIR"
cleanup(){ bash "$ROOT/scripts/stop-local.sh" >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM
bash "$ROOT/scripts/start-local.sh"
while true; do
  ok=1
  for f in "$STATE_DIR/controlplane.pid" "$STATE_DIR/agent.pid"; do
    if [[ -f "$f" ]]; then p="$(cat "$f" 2>/dev/null || true)"; [[ -n "$p" ]] && kill -0 "$p" 2>/dev/null || ok=0; else ok=0; fi
  done
  if [[ "$ok" != "1" ]]; then echo "NXYZ server child exited; supervisor will restart" >&2; exit 1; fi
  sleep 10
done
