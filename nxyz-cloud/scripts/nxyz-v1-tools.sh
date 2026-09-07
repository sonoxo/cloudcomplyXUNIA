#!/usr/bin/env bash
set -eo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
STATE="${NXYZ_LOCAL_STATE_DIR:-$ROOT/.nxyz}"
CONTROL="${NXYZ_CONTROL_PLANE:-}"
if [[ -z "$CONTROL" && -f "$STATE/remote-control-plane" ]]; then CONTROL="$(cat "$STATE/remote-control-plane")"; fi
CONTROL="${CONTROL:-http://127.0.0.1:8080}"; CONTROL="${CONTROL%/}"
TOKEN="${NXYZ_CLUSTER_TOKEN:-}"
if [[ -z "$TOKEN" && -f "$STATE/cluster.token" ]]; then TOKEN="$(cat "$STATE/cluster.token")"; fi
TOOLS="$STATE/tools"; BUILDS="$TOOLS/builds"
mkdir -p "$STATE" "$TOOLS" "$BUILDS"

fail(){ echo "❌ $*" >&2; exit 1; }
need(){ command -v "$1" >/dev/null 2>&1 || fail "Required command not found: $1"; }
safe(){ [[ "${1:-}" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || fail "Invalid name: ${1:-}"; }
json_escape(){ printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }
open_url(){ if [[ "$(uname -s)" == "Darwin" ]]; then open "$1" >/dev/null 2>&1 || true; else printf '%s\n' "$1"; fi; }
legacy(){ exec bash "$ROOT/scripts/nxyz-tools.sh" "$@"; }

api(){
  local method="$1" path="$2" body="${3:-}"
  if [[ -n "$TOKEN" ]]; then
    if [[ -n "$body" ]]; then curl -fsS -X "$method" -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' -d "$body" "$CONTROL$path";
    else curl -fsS -X "$method" -H "Authorization: Bearer $TOKEN" "$CONTROL$path"; fi
  else
    if [[ -n "$body" ]]; then curl -fsS -X "$method" -H 'content-type: application/json' -d "$body" "$CONTROL$path";
    else curl -fsS -X "$method" "$CONTROL$path"; fi
  fi
}

ensure_cloud(){
  curl -fsS "$CONTROL/healthz" >/dev/null 2>&1 && return 0
  if [[ "$CONTROL" != "http://127.0.0.1:8080" && "$CONTROL" != "http://localhost:8080" ]]; then fail "Remote NXYZ controller is unavailable: $CONTROL"; fi
  bash "$ROOT/scripts/start-local.sh"
}

pretty_services(){
  if command -v python3 >/dev/null 2>&1; then
    python3 -c 'import json,sys; d=json.load(sys.stdin); print("NAME\tSTATUS\tHEALTH\tENDPOINT\tNODE"); [print("{}\t{}\t{}\t{}\t{}".format(x.get("name",""),x.get("status",""),x.get("health",""),x.get("endpoint","-") or "-",x.get("node_id",""))) for x in d]'
  else cat; fi
}

services_cmd(){ ensure_cloud; api GET /api/v1/services | pretty_services; }

service_cmd(){
  local sub="${1:-list}"; shift || true
  case "$sub" in
    list|ls) services_cmd ;;
    open)
      local name="${1:-}" raw endpoint; [[ -n "$name" ]] || fail "Usage: nxyz service open NAME"; safe "$name"; ensure_cloud
      raw="$(api GET "/api/v1/services/$name")"; endpoint="$(printf '%s' "$raw" | sed -n 's/.*"endpoint":"\([^"]*\)".*/\1/p')"
      [[ -n "$endpoint" ]] || fail "Service $name does not have a reachable endpoint yet"; echo "🌐 $name -> $endpoint"; open_url "$endpoint"
      ;;
    proxy)
      local name="${1:-}"; [[ -n "$name" ]] || fail "Usage: nxyz service proxy NAME"; safe "$name"; ensure_cloud; echo "🔀 $CONTROL/service/$name/"; open_url "$CONTROL/service/$name/"
      ;;
    inspect) local name="${1:-}"; [[ -n "$name" ]] || fail "Usage: nxyz service inspect NAME"; safe "$name"; ensure_cloud; api GET "/api/v1/services/$name"; echo ;;
    *) echo "service: list | open NAME | proxy NAME | inspect NAME" ;;
  esac
}

storage_status(){
  local total used avail nxyz_kb nxyz_mb
  total="$(df -Pk "$ROOT" | awk 'NR==2 {print int($2/1024)}')"; used="$(df -Pk "$ROOT" | awk 'NR==2 {print int($3/1024)}')"; avail="$(df -Pk "$ROOT" | awk 'NR==2 {print int($4/1024)}')"
  nxyz_kb="$(du -sk "$STATE" 2>/dev/null | awk '{print $1}')"; nxyz_kb="${nxyz_kb:-0}"; nxyz_mb=$((nxyz_kb/1024))
  echo "💾 NXYZ STORAGE"; echo "   Disk total: ${total} MB"; echo "   Disk used:  ${used} MB"; echo "   Disk free:  ${avail} MB"; echo "   NXYZ used:  ${nxyz_mb} MB"
  if command -v podman >/dev/null 2>&1 && podman info >/dev/null 2>&1; then echo; echo "📦 Podman images/containers/volumes"; podman system df || true; fi
}
storage_cmd(){ local sub="${1:-list}"; shift || true; case "$sub" in status|capacity|usage) storage_status ;; *) legacy storage "$sub" "$@" ;; esac; }

create_workload(){
  local name="$1" image="$2" cpu="$3" mem="$4" port="$5" health="$6" publish=false body
  if [[ "$port" -gt 0 ]]; then publish=true; fi
  body="{\"name\":\"$(json_escape "$name")\",\"image\":\"$(json_escape "$image")\",\"cpu_millicores\":$cpu,\"memory_mb\":$mem,\"container_port\":$port,\"publish\":$publish,\"health_path\":\"$(json_escape "$health")\"}"
  api POST /api/v1/workloads "$body"
}

wait_for_endpoint(){
  local id="$1" endpoint="" raw=""
  for ((i=0;i<60;i++)); do
    raw="$(api GET "/api/v1/workloads/$id" 2>/dev/null || true)"; endpoint="$(printf '%s' "$raw" | sed -n 's/.*"endpoint":"\([^"]*\)".*/\1/p')"
    if [[ -n "$endpoint" ]]; then echo "🌐 Service online: $endpoint"; return 0; fi
    if printf '%s' "$raw" | grep -q '"status":"failed"'; then echo "$raw" >&2; return 1; fi
    sleep .5
  done
  echo "⚠️ Scheduled, but no endpoint yet. Run: nxyz services"
}

deploy_image_v1(){
  local name="${1:-}" image="${2:-}" cpu="${3:-250}" mem="${4:-256}" port="${5:-0}" health="${6:-}" result id
  [[ -n "$name" && -n "$image" ]] || fail "Usage: nxyz deploy image NAME IMAGE [CPU_M] [MEM_MB] [PORT] [HEALTH_PATH]"; safe "$name"; [[ "$port" =~ ^[0-9]+$ ]] || fail "PORT must be numeric"; ensure_cloud
  result="$(create_workload "$name" "$image" "$cpu" "$mem" "$port" "$health")"; echo "$result"; id="$(printf '%s' "$result" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
  if [[ "$port" -gt 0 && -n "$id" ]]; then wait_for_endpoint "$id" || true; fi
}

ensure_registry(){
  need podman; podman info >/dev/null 2>&1 || fail "Podman runtime unavailable"
  if podman container exists nxyz-registry >/dev/null 2>&1; then podman start nxyz-registry >/dev/null 2>&1 || true
  else podman volume inspect nxyz-registry-data >/dev/null 2>&1 || podman volume create nxyz-registry-data >/dev/null; podman run -d --name nxyz-registry --restart=unless-stopped -p 127.0.0.1:5000:5000 -v nxyz-registry-data:/var/lib/registry --label nxyz.tool=registry docker.io/library/registry:2 >/dev/null; fi
}

generate_dockerfile(){
  local src="$1" out="$2"; if [[ -f "$src/Dockerfile" ]]; then printf '%s\n' "$src/Dockerfile"; return; fi
  if [[ -f "$src/package.json" ]]; then cat >"$out" <<'EOF'
FROM docker.io/library/node:22-alpine
WORKDIR /app
COPY package*.json ./
RUN npm install --omit=dev
COPY . .
ENV HOST=0.0.0.0
CMD ["npm","start"]
EOF
  elif [[ -f "$src/go.mod" ]]; then cat >"$out" <<'EOF'
FROM docker.io/library/golang:1.23-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /app .
FROM docker.io/library/alpine:3.20
COPY --from=build /app /app
CMD ["/app"]
EOF
  elif [[ -f "$src/requirements.txt" ]]; then cat >"$out" <<'EOF'
FROM docker.io/library/python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
CMD ["python","app.py"]
EOF
  else fail "No Dockerfile and no supported Node/Go/Python project signature"; fi
  printf '%s\n' "$out"
}

deploy_git_v1(){
  local name="${1:-}" repo="${2:-}" cpu="${3:-500}" mem="${4:-512}" port="${5:-0}" health="${6:-}" dir src df image tag
  [[ -n "$name" && -n "$repo" ]] || fail "Usage: nxyz deploy git NAME GIT_URL [CPU_M] [MEM_MB] [PORT] [HEALTH_PATH]"; safe "$name"; need git; ensure_cloud; ensure_registry
  dir="$BUILDS/$name"; src="$dir/src"; mkdir -p "$dir"; if [[ -d "$src/.git" ]]; then git -C "$src" pull --ff-only; else rm -rf "$src"; git clone --depth=1 "$repo" "$src"; fi
  df="$(generate_dockerfile "$src" "$dir/Dockerfile.nxyz")"; tag="$(date +%Y%m%d%H%M%S)"; image="localhost:5000/nxyz/$name:$tag"
  echo "📦 Building $image"; podman build -t "$image" -f "$df" "$src"; podman push --tls-verify=false "$image"; deploy_image_v1 "$name" "$image" "$cpu" "$mem" "$port" "$health"
}
deploy_cmd(){ local sub="${1:-help}"; shift || true; case "$sub" in image) deploy_image_v1 "$@" ;; git) deploy_git_v1 "$@" ;; *) echo "deploy: image NAME IMAGE [CPU] [MEM] [PORT] [HEALTH] | git NAME URL [CPU] [MEM] [PORT] [HEALTH]" ;; esac; }

catalog_cmd(){
  local sub="${1:-list}"; shift || true
  case "$sub" in
    list|ls) legacy catalog list ;;
    install) local app="${1:-}" name="${2:-$1}"; case "$app" in nginx) deploy_image_v1 "$name" docker.io/library/nginx:alpine 250 128 80 / ;; httpd) deploy_image_v1 "$name" docker.io/library/httpd:alpine 250 128 80 / ;; whoami) deploy_image_v1 "$name" docker.io/traefik/whoami:latest 100 64 80 / ;; redis) deploy_image_v1 "$name" docker.io/library/redis:7-alpine 250 256 0 "" ;; *) fail "Unknown catalog app: $app" ;; esac ;;
    *) echo "catalog: list | install APP [NAME]" ;;
  esac
}

mesh_token(){ need openssl; if [[ ! -f "$STATE/cluster.token" ]]; then umask 077; openssl rand -hex 32 >"$STATE/cluster.token"; fi; cat "$STATE/cluster.token"; }
mesh_cmd(){
  local sub="${1:-status}"; shift || true
  case "$sub" in
    token) mesh_token ;;
    enable|controller) mesh_token >/dev/null; rm -f "$STATE/remote-control-plane"; touch "$STATE/mesh.enabled"; bash "$ROOT/scripts/stop-local.sh" >/dev/null 2>&1 || true; NXYZ_CONTROL_PLANE=http://127.0.0.1:8080 bash "$ROOT/scripts/start-local.sh"; echo "✅ Mesh controller enabled"; echo "Join another PC: nxyz mesh join http://<THIS-PC-IP>:8080 <TOKEN>"; echo "Show token: nxyz mesh token" ;;
    join) local remote="${1:-}" token="${2:-}"; [[ "$remote" == http://* || "$remote" == https://* ]] || fail "Usage: nxyz mesh join http://CONTROLLER:8080 TOKEN"; [[ -n "$token" ]] || fail "Mesh token required"; printf '%s\n' "${remote%/}" >"$STATE/remote-control-plane"; umask 077; printf '%s\n' "$token" >"$STATE/cluster.token"; touch "$STATE/mesh.enabled"; CONTROL="${remote%/}"; TOKEN="$token"; curl -fsS "$CONTROL/healthz" >/dev/null || fail "Cannot reach $CONTROL"; bash "$ROOT/scripts/stop-local.sh" >/dev/null 2>&1 || true; NXYZ_CONTROL_PLANE="$CONTROL" bash "$ROOT/scripts/start-local.sh"; echo "✅ Joined NXYZ mesh: $CONTROL" ;;
    disable|leave) rm -f "$STATE/mesh.enabled" "$STATE/remote-control-plane"; bash "$ROOT/scripts/stop-local.sh" >/dev/null 2>&1 || true; CONTROL=http://127.0.0.1:8080; NXYZ_CONTROL_PLANE="$CONTROL" bash "$ROOT/scripts/start-local.sh"; echo "✅ Mesh disabled; local-only mode restored" ;;
    status) ensure_cloud; echo "Controller: $CONTROL"; if [[ -f "$STATE/mesh.enabled" ]]; then echo "Mode: mesh"; else echo "Mode: local-only"; fi; api GET /api/v1/nodes; echo ;;
    *) echo "mesh: enable | join URL TOKEN | status | token | disable" ;;
  esac
}

tools_status(){ cat <<'EOF'
NXYZ CLOUD v1 TOOL PLANE
  compute     ✅ rootless Podman scheduler
  services    ✅ dynamic port publishing + health
  ingress     ✅ built-in service reverse proxy
  storage     ✅ buckets + capacity/status
  database    ✅ PostgreSQL lifecycle
  registry    ✅ private OCI registry
  git/deploy  ✅ Git → OCI → registry → scheduler
  secrets     ✅ encrypted local secrets
  backup      ✅ file/database backup + restore
  monitoring  ✅ Prometheus + runtime metrics
  ai          ✅ llama.cpp local API hook
  agents      ✅ workload-based agent launcher
  mesh        ✅ controller + worker enrollment
  dns/proxy   ✅ private service records/routes
EOF
}

doctor_cmd(){
  echo "🩺 NXYZ CLOUD DOCTOR"; local failures=0 c
  for c in go curl podman git openssl; do if command -v "$c" >/dev/null 2>&1; then echo "✅ $c"; else echo "❌ $c missing"; failures=$((failures+1)); fi; done
  if command -v podman >/dev/null 2>&1 && podman info >/dev/null 2>&1; then echo "✅ Podman runtime ready"; else echo "❌ Podman runtime unavailable"; failures=$((failures+1)); fi
  if curl -fsS "$CONTROL/healthz" >/dev/null 2>&1; then echo "✅ Control plane $CONTROL"; else echo "❌ Control plane $CONTROL"; failures=$((failures+1)); fi
  storage_status; echo; if [[ "$failures" == "0" ]]; then echo "✅ NXYZ is operational"; else echo "⚠️ $failures check(s) need attention"; return 1; fi
}
info_cmd(){ ensure_cloud; echo "☁️ NXYZ CLOUD v1"; api GET /api/v1/system; echo; echo; services_cmd; }

cmd="${1:-tools}"; shift || true
case "$cmd" in
  tools) tools_status ;;
  storage) storage_cmd "$@" ;;
  deploy) deploy_cmd "$@" ;;
  services) services_cmd ;;
  service) service_cmd "$@" ;;
  catalog|apps) catalog_cmd "$@" ;;
  mesh) mesh_cmd "$@" ;;
  doctor) doctor_cmd ;;
  info) info_cmd ;;
  *) legacy "$cmd" "$@" ;;
esac
