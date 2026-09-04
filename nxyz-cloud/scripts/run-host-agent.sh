#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if ! command -v go >/dev/null 2>&1; then
  echo "NXYZ host agent requires Go 1.23+ in PATH." >&2
  exit 1
fi

runtime="${NXYZ_RUNTIME:-podman}"
if ! command -v "$runtime" >/dev/null 2>&1; then
  echo "NXYZ execution runtime '$runtime' was not found." >&2
  echo "Install free Podman first, then rerun this script." >&2
  exit 1
fi

if [[ "$(uname -s)" == "Darwin" && "$runtime" == "podman" ]]; then
  if ! podman info >/dev/null 2>&1; then
    echo "Starting the local Podman machine..."
    if ! podman machine start >/dev/null 2>&1; then
      echo "No usable Podman machine found; initializing one..."
      podman machine init --now >/dev/null
    fi
    for ((i=0; i<40; i++)); do
      podman info >/dev/null 2>&1 && break
      sleep 0.5
    done
    if ! podman info >/dev/null 2>&1; then
      echo "Podman machine did not become ready." >&2
      podman machine list || true
      exit 1
    fi
  fi
fi

cp="${NXYZ_CONTROL_PLANE:-http://127.0.0.1:8080}"
token="${NXYZ_CLUSTER_TOKEN:-}"
health() {
  if [[ -n "$token" ]]; then
    curl -fsS -H "Authorization: Bearer $token" "$cp/healthz" >/dev/null 2>&1
  else
    curl -fsS "$cp/healthz" >/dev/null 2>&1
  fi
}

# If this agent points at the local NXYZ endpoint and nothing is listening,
# recover automatically by starting the control plane first.
if [[ "$cp" == http://127.0.0.1:8080 || "$cp" == http://localhost:8080 ]]; then
  if ! health; then
    state_dir="${NXYZ_LOCAL_STATE_DIR:-$ROOT/.nxyz}"
    mkdir -p "$state_dir/bin"
    echo "Local NXYZ control plane is offline; starting it automatically..."
    go build -o "$state_dir/bin/nxyz-controlplane" ./cmd/controlplane
    nohup env \
      NXYZ_LISTEN="127.0.0.1:8080" \
      NXYZ_STATE="$state_dir/state.json" \
      NXYZ_CLUSTER_TOKEN="$token" \
      "$state_dir/bin/nxyz-controlplane" >"$state_dir/controlplane.log" 2>&1 &
    echo $! > "$state_dir/controlplane.pid"
    for ((i=0; i<60; i++)); do
      health && break
      sleep 0.25
    done
    if ! health; then
      echo "Unable to start the local NXYZ control plane. Last log lines:" >&2
      tail -n 40 "$state_dir/controlplane.log" >&2 || true
      exit 1
    fi
  fi
fi

export NXYZ_CONTROL_PLANE="$cp"
export NXYZ_EXECUTION_ENABLED="${NXYZ_EXECUTION_ENABLED:-true}"
export NXYZ_RUNTIME="$runtime"

echo "Starting NXYZ rootless execution agent with runtime: $runtime"
exec go run ./cmd/agent
