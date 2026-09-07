#!/usr/bin/env bash
set -eo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
TMP="${RUNNER_TEMP:-/tmp}/nxyz-v1-smoke-$$"
mkdir -p "$TMP"
go build -o "$TMP/controlplane" ./cmd/controlplane
NXYZ_LISTEN=127.0.0.1:18080 NXYZ_STATE="$TMP/state.json" "$TMP/controlplane" >"$TMP/control.log" 2>&1 &
PID=$!
trap 'kill "$PID" 2>/dev/null || true; rm -rf "$TMP"' EXIT
for ((i=0;i<40;i++)); do curl -fsS http://127.0.0.1:18080/healthz >/dev/null 2>&1 && break; sleep .1; done
curl -fsS http://127.0.0.1:18080/healthz | grep -q '1.0.0'
curl -fsS -X POST http://127.0.0.1:18080/api/v1/nodes/register -H 'content-type: application/json' -d '{"id":"ci","name":"CI Node","address":"127.0.0.1","capacity_cpu_millicores":2000,"capacity_memory_mb":2048,"capacity_disk_mb":4096}' >/dev/null
RESULT="$(curl -fsS -X POST http://127.0.0.1:18080/api/v1/workloads -H 'content-type: application/json' -d '{"name":"web","image":"nginx:alpine","cpu_millicores":250,"memory_mb":128,"container_port":80,"publish":true,"health_path":"/"}')"
ID="$(printf '%s' "$RESULT" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
[[ -n "$ID" ]]
curl -fsS -X POST "http://127.0.0.1:18080/api/v1/workloads/$ID/status" -H 'content-type: application/json' -d '{"node_id":"ci","status":"running","host_port":49152,"endpoint":"http://127.0.0.1:49152","health":"healthy"}' >/dev/null
curl -fsS http://127.0.0.1:18080/api/v1/services | grep -q 'http://127.0.0.1:49152'
curl -fsS http://127.0.0.1:18080/api/v1/system | grep -q '"published_services":1'
echo '✅ NXYZ v1 API smoke passed'
