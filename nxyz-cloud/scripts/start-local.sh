#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

STATE_DIR="${NXYZ_LOCAL_STATE_DIR:-$ROOT/.nxyz}"
BIN_DIR="$STATE_DIR/bin"
CONTROL_LOG="$STATE_DIR/controlplane.log"
AGENT_LOG="$STATE_DIR/agent.log"
CONTROL_PID="$STATE_DIR/controlplane.pid"
AGENT_PID="$STATE_DIR/agent.pid"
CONTROL_PLANE="${NXYZ_CONTROL_PLANE:-http://127.0.0.1:8080}"
LISTEN="${NXYZ_LISTEN:-127.0.0.1:8080}"
TOKEN="${NXYZ_CLUSTER_TOKEN:-}"

mkdir -p "$BIN_DIR"
if [[ -z "$TOKEN" && -f "$STATE_DIR/cluster.token" ]]; then TOKEN="$(cat "$STATE_DIR/cluster.token")"; fi

need() {
  local cmd="$1"
  if command -v "$cmd" >/dev/null 2>&1; then return 0; fi
  if [[ "$(uname -s)" == "Darwin" ]] && command -v brew >/dev/null 2>&1; then
    echo "Installing missing free dependency: $cmd"
    brew install "$cmd"
    return 0
  fi
  echo "Missing required command: $cmd" >&2
  return 1
}

need go
need curl
need podman

if [[ "$(uname -s)" == "Darwin" ]]; then
  if ! podman info >/dev/null 2>&1; then
    echo "Starting Podman machine..."
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

echo "Building NXYZ Cloud binaries..."
go build -o "$BIN_DIR/nxyz-controlplane" ./cmd/controlplane
go build -o "$BIN_DIR/nxyz-agent" ./cmd/agent

pid_alive() {
  [[ -f "$1" ]] || return 1
  local p
  p="$(cat "$1" 2>/dev/null || true)"
  [[ -n "$p" ]] && kill -0 "$p" 2>/dev/null
}

health() {
  if [[ -n "$TOKEN" ]]; then
    curl -fsS -H "Authorization: Bearer $TOKEN" "$CONTROL_PLANE/healthz" >/dev/null 2>&1
  else
    curl -fsS "$CONTROL_PLANE/healthz" >/dev/null 2>&1
  fi
}

nodes_json() {
  if [[ -n "$TOKEN" ]]; then
    curl -fsS -H "Authorization: Bearer $TOKEN" "$CONTROL_PLANE/api/v1/nodes"
  else
    curl -fsS "$CONTROL_PLANE/api/v1/nodes"
  fi
}

if ! health; then
  if pid_alive "$CONTROL_PID"; then
    kill "$(cat "$CONTROL_PID")" 2>/dev/null || true
    sleep 0.5
  fi
  rm -f "$CONTROL_PID"
  echo "Starting NXYZ control plane on $LISTEN..."
  nohup env \
    NXYZ_LISTEN="$LISTEN" \
    NXYZ_STATE="$STATE_DIR/state.json" \
    NXYZ_CLUSTER_TOKEN="$TOKEN" \
    "$BIN_DIR/nxyz-controlplane" >"$CONTROL_LOG" 2>&1 &
  echo $! > "$CONTROL_PID"

  for ((i=0; i<60; i++)); do
    health && break
    sleep 0.25
  done
  if ! health; then
    echo "NXYZ control plane did not become healthy. Last log lines:" >&2
    tail -n 40 "$CONTROL_LOG" >&2 || true
    exit 1
  fi
fi

if [[ "$(uname -s)" == "Darwin" ]]; then
  CORES="$(sysctl -n hw.logicalcpu 2>/dev/null || echo 4)"
  MEM_BYTES="$(sysctl -n hw.memsize 2>/dev/null || echo 8589934592)"
  MEM_MB=$((MEM_BYTES / 1024 / 1024))
else
  CORES="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)"
  MEM_MB="$(awk '/MemTotal/ {print int($2/1024)}' /proc/meminfo 2>/dev/null || echo 8192)"
fi
CPU_DEFAULT=$((CORES * 1000 * 3 / 4))
MEM_DEFAULT=$((MEM_MB * 3 / 4))
(( CPU_DEFAULT < 1000 )) && CPU_DEFAULT=1000
(( MEM_DEFAULT < 1024 )) && MEM_DEFAULT=1024

NODE_ID="${NXYZ_NODE_ID:-$(hostname | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9-' '-')-exec}"
NODE_NAME="${NXYZ_NODE_NAME:-Local Rootless Compute}"
NODE_CPU="${NXYZ_NODE_CPU:-$CPU_DEFAULT}"
NODE_MEMORY="${NXYZ_NODE_MEMORY_MB:-$MEM_DEFAULT}"

if pid_alive "$AGENT_PID"; then
  kill "$(cat "$AGENT_PID")" 2>/dev/null || true
  sleep 0.5
fi
rm -f "$AGENT_PID"

echo "Starting NXYZ rootless Podman execution agent..."
nohup env \
  NXYZ_CONTROL_PLANE="$CONTROL_PLANE" \
  NXYZ_CLUSTER_TOKEN="$TOKEN" \
  NXYZ_NODE_ID="$NODE_ID" \
  NXYZ_NODE_NAME="$NODE_NAME" \
  NXYZ_NODE_CPU="$NODE_CPU" \
  NXYZ_NODE_MEMORY_MB="$NODE_MEMORY" \
  NXYZ_EXECUTION_ENABLED="true" \
  NXYZ_RUNTIME="podman" \
  "$BIN_DIR/nxyz-agent" >"$AGENT_LOG" 2>&1 &
echo $! > "$AGENT_PID"

REGISTERED=0
for ((i=0; i<60; i++)); do
  if nodes_json 2>/dev/null | grep -q "\"$NODE_ID\""; then
    REGISTERED=1
    break
  fi
  sleep 0.25
done

if [[ "$REGISTERED" != "1" ]]; then
  echo "Agent did not register. Last agent log lines:" >&2
  tail -n 40 "$AGENT_LOG" >&2 || true
  exit 1
fi

echo
echo "✅ NXYZ Cloud is online"
echo "   Dashboard: $CONTROL_PLANE/"
echo "   Health:    $CONTROL_PLANE/healthz"
echo "   Listen:    $LISTEN"
echo "   Node:      $NODE_NAME ($NODE_ID)"
echo "   CPU:       ${NODE_CPU} millicores"
echo "   Memory:    ${NODE_MEMORY} MB"
echo "   Runtime:   rootless Podman"
echo "   Logs:      $CONTROL_LOG"
echo "              $AGENT_LOG"
echo
if [[ "$(uname -s)" == "Darwin" ]]; then
  open "$CONTROL_PLANE/" >/dev/null 2>&1 || true
fi
