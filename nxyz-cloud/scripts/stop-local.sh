#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STATE_DIR="${NXYZ_LOCAL_STATE_DIR:-$ROOT/.nxyz}"

stop_pid() {
  local file="$1" name="$2"
  if [[ -f "$file" ]]; then
    local pid
    pid="$(cat "$file" 2>/dev/null || true)"
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      echo "Stopping $name (pid $pid)..."
      kill "$pid" 2>/dev/null || true
      for _ in $(seq 1 20); do
        kill -0 "$pid" 2>/dev/null || break
        sleep 0.1
      done
      kill -9 "$pid" 2>/dev/null || true
    fi
    rm -f "$file"
  fi
}

stop_pid "$STATE_DIR/agent.pid" "NXYZ agent"
stop_pid "$STATE_DIR/controlplane.pid" "NXYZ control plane"
echo "✅ NXYZ local services stopped"
