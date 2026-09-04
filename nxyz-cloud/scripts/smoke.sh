#!/usr/bin/env sh
set -eu
BASE="${NXYZ_URL:-http://127.0.0.1:8080}"
echo "[1/4] health"
curl -fsS "$BASE/healthz"
echo "\n[2/4] system"
curl -fsS "$BASE/api/v1/system"
echo "\n[3/4] nodes"
curl -fsS "$BASE/api/v1/nodes"
echo "\n[4/4] schedule test workload"
curl -fsS -X POST "$BASE/api/v1/workloads" -H 'content-type: application/json' -d '{"name":"smoke-test","image":"busybox:1.36","cpu_millicores":100,"memory_mb":64}'
echo "\nNXYZ Cloud smoke test passed."
