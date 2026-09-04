#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
STATE="${NXYZ_LOCAL_STATE_DIR:-$ROOT/.nxyz}"
CONTROL="${NXYZ_CONTROL_PLANE:-http://127.0.0.1:8080}"
TOOLS="$STATE/tools"
STORAGE="$TOOLS/storage"
DATABASES="$TOOLS/databases"
BACKUPS="$TOOLS/backups"
SECRETS="$TOOLS/secrets"
GITROOT="$TOOLS/git"
BUILDS="$TOOLS/builds"
DNSROOT="$TOOLS/dns"
PROXYROOT="$TOOLS/proxy"
AICFG="$TOOLS/ai"
mkdir -p "$STORAGE" "$DATABASES" "$BACKUPS" "$SECRETS" "$GITROOT" "$BUILDS" "$DNSROOT" "$PROXYROOT" "$AICFG"
chmod 700 "$SECRETS" 2>/dev/null || true

TOKEN="${NXYZ_CLUSTER_TOKEN:-}"
if [[ -z "$TOKEN" && -f "$STATE/cluster.token" ]]; then TOKEN="$(cat "$STATE/cluster.token")"; fi

fail() { echo "❌ $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "Required command not found: $1"; }
safe() { [[ "$1" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || fail "Invalid name: $1"; }
open_url() { if [[ "$(uname -s)" == "Darwin" ]]; then open "$1" >/dev/null 2>&1 || true; else echo "$1"; fi; }

api() {
  local method="$1" path="$2" body="${3:-}"
  if [[ -n "$TOKEN" ]]; then
    if [[ -n "$body" ]]; then curl -fsS -X "$method" -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' -d "$body" "$CONTROL$path";
    else curl -fsS -X "$method" -H "Authorization: Bearer $TOKEN" "$CONTROL$path"; fi
  else
    if [[ -n "$body" ]]; then curl -fsS -X "$method" -H 'content-type: application/json' -d "$body" "$CONTROL$path";
    else curl -fsS -X "$method" "$CONTROL$path"; fi
  fi
}

ensure_cloud() {
  if ! curl -fsS "$CONTROL/healthz" >/dev/null 2>&1; then
    echo "☁️  Starting NXYZ Cloud..."
    bash "$ROOT/scripts/start-local.sh"
  fi
}

ensure_podman() {
  need podman
  if ! podman info >/dev/null 2>&1; then
    if [[ "$(uname -s)" == "Darwin" ]]; then podman machine start >/dev/null 2>&1 || podman machine init --now >/dev/null; fi
  fi
  podman info >/dev/null 2>&1 || fail "Podman runtime is unavailable"
}

ensure_registry() {
  ensure_podman
  if podman container exists nxyz-registry >/dev/null 2>&1; then
    podman start nxyz-registry >/dev/null 2>&1 || true
  else
    podman volume inspect nxyz-registry-data >/dev/null 2>&1 || podman volume create nxyz-registry-data >/dev/null
    podman run -d --name nxyz-registry --restart=unless-stopped \
      -p 127.0.0.1:5000:5000 -v nxyz-registry-data:/var/lib/registry \
      --label nxyz.tool=registry docker.io/library/registry:2 >/dev/null
  fi
  echo "✅ NXYZ Registry: localhost:5000"
}

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

storage_cmd() {
  local sub="${1:-help}"; shift || true
  case "$sub" in
    create|init) local b="${1:-}"; [[ -n "$b" ]] || fail "Usage: nxyz storage create BUCKET"; safe "$b"; mkdir -p "$STORAGE/$b"; echo "✅ bucket $b" ;;
    put) local b="${1:-}" src="${2:-}" key="${3:-}"; [[ -n "$b" && -f "$src" ]] || fail "Usage: nxyz storage put BUCKET FILE [KEY]"; safe "$b"; mkdir -p "$STORAGE/$b"; [[ -n "$key" ]] || key="$(basename "$src")"; safe "$key"; cp "$src" "$STORAGE/$b/$key"; echo "✅ nxyz://$b/$key" ;;
    get) local b="${1:-}" key="${2:-}" dst="${3:-.}"; [[ -f "$STORAGE/$b/$key" ]] || fail "Object not found"; cp "$STORAGE/$b/$key" "$dst"; echo "✅ copied to $dst" ;;
    ls|list) local b="${1:-}"; if [[ -n "$b" ]]; then safe "$b"; find "$STORAGE/$b" -maxdepth 1 -type f -print 2>/dev/null | sed "s#^$STORAGE/$b/##"; else find "$STORAGE" -mindepth 1 -maxdepth 1 -type d -print | sed "s#^$STORAGE/##"; fi ;;
    rm|delete) local b="${1:-}" key="${2:-}"; [[ -n "$b" ]] || fail "Usage: nxyz storage delete BUCKET [KEY]"; safe "$b"; if [[ -n "$key" ]]; then safe "$key"; rm -f "$STORAGE/$b/$key"; else rm -rf "$STORAGE/$b"; fi ;;
    *) echo "storage: create | put | get | list | delete" ;;
  esac
}

deploy_image() {
  local name="${1:-}" image="${2:-}" cpu="${3:-250}" mem="${4:-256}"
  [[ -n "$name" && -n "$image" ]] || fail "Usage: nxyz deploy image NAME IMAGE [CPU_M] [MEM_MB]"
  safe "$name"; ensure_cloud
  local body
  body="{\"name\":\"$(json_escape "$name")\",\"image\":\"$(json_escape "$image")\",\"cpu_millicores\":$cpu,\"memory_mb\":$mem}"
  api POST /api/v1/workloads "$body"; echo
}

generate_dockerfile() {
  local src="$1" out="$2"
  if [[ -f "$src/Dockerfile" ]]; then printf '%s\n' "$src/Dockerfile"; return; fi
  if [[ -f "$src/package.json" ]]; then
    cat >"$out" <<'EOF'
FROM docker.io/library/node:22-alpine
WORKDIR /app
COPY package*.json ./
RUN npm install --omit=dev
COPY . .
CMD ["npm","start"]
EOF
  elif [[ -f "$src/go.mod" ]]; then
    cat >"$out" <<'EOF'
FROM docker.io/library/golang:1.23-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /app .
FROM docker.io/library/alpine:3.20
COPY --from=build /app /app
CMD ["/app"]
EOF
  elif [[ -f "$src/requirements.txt" ]]; then
    cat >"$out" <<'EOF'
FROM docker.io/library/python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
CMD ["python","app.py"]
EOF
  else
    fail "No Dockerfile and no supported Node/Go/Python project signature"
  fi
  printf '%s\n' "$out"
}

deploy_git() {
  local name="${1:-}" repo="${2:-}" cpu="${3:-500}" mem="${4:-512}"
  [[ -n "$name" && -n "$repo" ]] || fail "Usage: nxyz deploy git NAME GIT_URL [CPU_M] [MEM_MB]"
  safe "$name"; need git; ensure_registry
  local dir="$BUILDS/$name" src="$BUILDS/$name/src" df image tag
  mkdir -p "$dir"
  if [[ -d "$src/.git" ]]; then git -C "$src" pull --ff-only; else rm -rf "$src"; git clone --depth=1 "$repo" "$src"; fi
  df="$(generate_dockerfile "$src" "$dir/Dockerfile.nxyz")"
  tag="$(date +%Y%m%d%H%M%S)"
  image="localhost:5000/nxyz/$name:$tag"
  echo "📦 Building $image"
  podman build -t "$image" -f "$df" "$src"
  podman push --tls-verify=false "$image"
  deploy_image "$name" "$image" "$cpu" "$mem"
}

deploy_cmd() {
  local sub="${1:-}"; shift || true
  case "$sub" in image) deploy_image "$@" ;; git) deploy_git "$@" ;; *) echo "deploy: image NAME IMAGE [CPU] [MEM] | git NAME URL [CPU] [MEM]" ;; esac
}

db_cmd() {
  local sub="${1:-help}"; shift || true; ensure_podman
  case "$sub" in
    create)
      local name="${1:-}"; [[ -n "$name" ]] || fail "Usage: nxyz db create NAME"; safe "$name"
      local c="nxyz-db-$name" pass info port
      if podman container exists "$c" >/dev/null 2>&1; then fail "Database already exists: $name"; fi
      pass="$(openssl rand -hex 18 2>/dev/null || date +%s%N)"
      podman volume inspect "$c-data" >/dev/null 2>&1 || podman volume create "$c-data" >/dev/null
      podman run -d --name "$c" --restart=unless-stopped -p 127.0.0.1::5432 \
        -e POSTGRES_PASSWORD="$pass" -v "$c-data:/var/lib/postgresql/data" \
        --label nxyz.tool=database --label "nxyz.name=$name" docker.io/library/postgres:16-alpine >/dev/null
      sleep 1
      port="$(podman port "$c" 5432/tcp | sed -n '1s/.*://p')"
      info="$DATABASES/$name.env"; umask 077; printf 'NAME=%s\nHOST=127.0.0.1\nPORT=%s\nUSER=postgres\nPASSWORD=%s\nDATABASE=postgres\n' "$name" "$port" "$pass" >"$info"
      echo "✅ PostgreSQL $name on 127.0.0.1:$port"; echo "🔐 Credentials: $info"
      ;;
    list|ls) podman ps -a --filter label=nxyz.tool=database --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}' ;;
    url) local name="${1:-}"; [[ -f "$DATABASES/$name.env" ]] || fail "Unknown database"; . "$DATABASES/$name.env"; printf 'postgresql://%s:%s@%s:%s/%s\n' "$USER" "$PASSWORD" "$HOST" "$PORT" "$DATABASE" ;;
    stop) local name="${1:-}"; safe "$name"; podman stop "nxyz-db-$name" >/dev/null; echo "✅ stopped $name" ;;
    start) local name="${1:-}"; safe "$name"; podman start "nxyz-db-$name" >/dev/null; echo "✅ started $name" ;;
    delete|rm) local name="${1:-}"; safe "$name"; podman rm -f "nxyz-db-$name" >/dev/null 2>&1 || true; podman volume rm "nxyz-db-$name-data" >/dev/null 2>&1 || true; rm -f "$DATABASES/$name.env"; echo "✅ deleted $name" ;;
    *) echo "db: create | list | url | start | stop | delete" ;;
  esac
}

monitor_cmd() {
  ensure_cloud
  echo "☁️ NXYZ CONTROL PLANE"; api GET /api/v1/summary; echo
  if command -v podman >/dev/null 2>&1 && podman info >/dev/null 2>&1; then
    echo; echo "📦 NXYZ CONTAINERS"; podman ps --filter label=nxyz.managed=true --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}' || true
    echo; echo "📊 RESOURCE SNAPSHOT"; podman stats --no-stream --format 'table {{.Name}}\t{{.CPU}}\t{{.MemUsage}}' 2>/dev/null || true
  fi
}

logs_cmd() {
  local id="${1:-}"
  if [[ -z "$id" ]]; then tail -n 60 "$STATE/controlplane.log" 2>/dev/null || true; echo "--- agent ---"; tail -n 60 "$STATE/agent.log" 2>/dev/null || true; return; fi
  ensure_podman; podman logs --tail 100 "nxyz-$id"
}

secret_key() {
  local key="$SECRETS/master.key"
  if [[ ! -f "$key" ]]; then umask 077; openssl rand -hex 32 >"$key"; fi
  printf '%s\n' "$key"
}

secrets_cmd() {
  need openssl; local sub="${1:-help}"; shift || true
  case "$sub" in
    set) local name="${1:-}" value="${2:-}"; [[ -n "$name" ]] || fail "Usage: nxyz secrets set NAME VALUE"; safe "$name"; local key; key="$(secret_key)"; printf '%s' "$value" | openssl enc -aes-256-cbc -pbkdf2 -salt -pass "file:$key" -out "$SECRETS/$name.enc"; chmod 600 "$SECRETS/$name.enc"; echo "✅ secret $name" ;;
    get) local name="${1:-}"; safe "$name"; local key; key="$(secret_key)"; openssl enc -d -aes-256-cbc -pbkdf2 -pass "file:$key" -in "$SECRETS/$name.enc" ; echo ;;
    list|ls) find "$SECRETS" -name '*.enc' -maxdepth 1 -print | sed -e "s#^$SECRETS/##" -e 's/\.enc$//' ;;
    delete|rm) local name="${1:-}"; safe "$name"; rm -f "$SECRETS/$name.enc" ;;
    *) echo "secrets: set NAME VALUE | get NAME | list | delete NAME" ;;
  esac
}

backup_cmd() {
  local sub="${1:-create}"; shift || true; ensure_podman
  case "$sub" in
    create)
      local name="${1:-$(date +%Y%m%d-%H%M%S)}" dir="$BACKUPS/$name"; safe "$name"; mkdir -p "$dir/db"
      for c in $(podman ps --filter label=nxyz.tool=database --format '{{.Names}}'); do podman exec "$c" pg_dumpall -U postgres >"$dir/db/${c#nxyz-db-}.sql" || true; done
      tar -czf "$dir/files.tgz" -C "$TOOLS" storage git dns proxy 2>/dev/null || true
      echo "✅ backup $name -> $dir"
      ;;
    list|ls) find "$BACKUPS" -mindepth 1 -maxdepth 1 -type d -print | sed "s#^$BACKUPS/##" ;;
    restore)
      local name="${1:-}"; [[ -d "$BACKUPS/$name" ]] || fail "Backup not found"; local dir="$BACKUPS/$name"
      [[ -f "$dir/files.tgz" ]] && tar -xzf "$dir/files.tgz" -C "$TOOLS"
      for f in "$dir"/db/*.sql; do [[ -f "$f" ]] || continue; local dbn c; dbn="$(basename "$f" .sql)"; c="nxyz-db-$dbn"; podman container exists "$c" >/dev/null 2>&1 || db_cmd create "$dbn"; podman start "$c" >/dev/null 2>&1 || true; cat "$f" | podman exec -i "$c" psql -U postgres >/dev/null; done
      echo "✅ restored $name"
      ;;
    *) echo "backup: create [NAME] | list | restore NAME" ;;
  esac
}

registry_cmd() {
  local sub="${1:-start}"; shift || true
  case "$sub" in
    start) ensure_registry ;;
    stop) ensure_podman; podman stop nxyz-registry >/dev/null 2>&1 || true ;;
    list|ls) ensure_registry; curl -fsS http://127.0.0.1:5000/v2/_catalog; echo ;;
    *) echo "registry: start | stop | list" ;;
  esac
}

git_cmd() {
  need git; local sub="${1:-list}"; shift || true
  case "$sub" in
    create) local name="${1:-}"; [[ -n "$name" ]] || fail "Usage: nxyz git create NAME"; safe "$name"; git init --bare "$GITROOT/$name.git" >/dev/null; echo "✅ $GITROOT/$name.git" ;;
    list|ls) find "$GITROOT" -mindepth 1 -maxdepth 1 -name '*.git' -type d -print | sed -e "s#^$GITROOT/##" -e 's/\.git$//' ;;
    path) local name="${1:-}"; safe "$name"; printf '%s\n' "$GITROOT/$name.git" ;;
    *) echo "git: create NAME | list | path NAME" ;;
  esac
}

ai_cmd() {
  local sub="${1:-status}"; shift || true
  case "$sub" in
    install) if command -v llama-server >/dev/null 2>&1; then echo "✅ llama.cpp already installed"; elif [[ "$(uname -s)" == "Darwin" ]] && command -v brew >/dev/null 2>&1; then brew install llama.cpp; else fail "Install llama.cpp/llama-server and retry"; fi ;;
    serve)
      need llama-server; local model="${1:-}" port="${2:-8090}"; [[ -f "$model" ]] || fail "Usage: nxyz ai serve /path/model.gguf [PORT]"
      nohup llama-server -m "$model" --host 127.0.0.1 --port "$port" >"$AICFG/server.log" 2>&1 & echo $! >"$AICFG/server.pid"; printf '%s\n' "$model" >"$AICFG/model"; echo "✅ NXYZ AI API: http://127.0.0.1:$port/v1" ;;
    stop) [[ -f "$AICFG/server.pid" ]] && kill "$(cat "$AICFG/server.pid")" 2>/dev/null || true; rm -f "$AICFG/server.pid" ;;
    status) if [[ -f "$AICFG/server.pid" ]] && kill -0 "$(cat "$AICFG/server.pid")" 2>/dev/null; then echo "✅ NXYZ AI running (pid $(cat "$AICFG/server.pid"))"; else echo "○ NXYZ AI stopped"; fi ;;
    *) echo "ai: install | serve MODEL.gguf [PORT] | status | stop" ;;
  esac
}

agents_cmd() {
  local sub="${1:-list}"; shift || true
  case "$sub" in
    run) local name="${1:-}" image="${2:-}" cpu="${3:-500}" mem="${4:-512}"; [[ -n "$name" && -n "$image" ]] || fail "Usage: nxyz agents run NAME IMAGE [CPU] [MEM]"; deploy_image "agent-$name" "$image" "$cpu" "$mem" ;;
    list|ls) ensure_cloud; api GET /api/v1/workloads | tr '{' '\n' | grep '"name":"agent-' || true ;;
    *) echo "agents: run NAME IMAGE [CPU] [MEM] | list" ;;
  esac
}

dns_cmd() {
  local file="$DNSROOT/records" sub="${1:-list}"; shift || true; touch "$file"
  case "$sub" in
    add) local name="${1:-}" addr="${2:-}"; [[ -n "$name" && -n "$addr" ]] || fail "Usage: nxyz dns add NAME ADDRESS"; safe "$name"; grep -v "^$name " "$file" >"$file.tmp" || true; printf '%s %s\n' "$name" "$addr" >>"$file.tmp"; mv "$file.tmp" "$file"; echo "✅ $name -> $addr" ;;
    resolve) local name="${1:-}"; awk -v n="$name" '$1==n{print $2}' "$file" ;;
    list|ls) cat "$file" ;;
    delete|rm) local name="${1:-}"; grep -v "^$name " "$file" >"$file.tmp" || true; mv "$file.tmp" "$file" ;;
    *) echo "dns: add NAME ADDRESS | resolve NAME | list | delete NAME" ;;
  esac
}

terminal_cmd() {
  local id="${1:-}"; [[ -n "$id" ]] || fail "Usage: nxyz terminal WORKLOAD_ID"; ensure_podman
  local c="nxyz-$id"; podman container exists "$c" >/dev/null 2>&1 || fail "Container $c not found"
  podman exec -it "$c" sh -lc 'command -v bash >/dev/null 2>&1 && exec bash || exec sh'
}

catalog_cmd() {
  local sub="${1:-list}"; shift || true
  case "$sub" in
    list|ls) cat <<'EOF'
nginx     docker.io/library/nginx:alpine       250m / 128MB
redis     docker.io/library/redis:7-alpine     250m / 256MB
httpd     docker.io/library/httpd:alpine       250m / 128MB
whoami    docker.io/traefik/whoami:latest      100m / 64MB
EOF
      ;;
    install) local app="${1:-}" name="${2:-$1}"; case "$app" in nginx) deploy_image "$name" docker.io/library/nginx:alpine 250 128 ;; redis) deploy_image "$name" docker.io/library/redis:7-alpine 250 256 ;; httpd) deploy_image "$name" docker.io/library/httpd:alpine 250 128 ;; whoami) deploy_image "$name" docker.io/traefik/whoami:latest 100 64 ;; *) fail "Unknown catalog app: $app" ;; esac ;;
    *) echo "catalog: list | install APP [NAME]" ;;
  esac
}

proxy_cmd() {
  local sub="${1:-list}"; shift || true; local cfg="$PROXYROOT/routes"
  touch "$cfg"
  case "$sub" in
    add) local name="${1:-}" upstream="${2:-}"; [[ -n "$name" && -n "$upstream" ]] || fail "Usage: nxyz proxy add NAME UPSTREAM_URL"; safe "$name"; grep -v "^$name " "$cfg" >"$cfg.tmp" || true; printf '%s %s\n' "$name" "$upstream" >>"$cfg.tmp"; mv "$cfg.tmp" "$cfg"; echo "✅ route recorded: $name -> $upstream" ;;
    list|ls) cat "$cfg" ;;
    open) local name="${1:-}" url; url="$(awk -v n="$name" '$1==n{print $2}' "$cfg")"; [[ -n "$url" ]] || fail "Route not found"; open_url "$url" ;;
    *) echo "proxy: add NAME UPSTREAM_URL | list | open NAME" ;;
  esac
}

mesh_cmd() {
  local sub="${1:-status}"; shift || true
  case "$sub" in
    token) if [[ ! -f "$STATE/cluster.token" ]]; then umask 077; openssl rand -hex 32 >"$STATE/cluster.token"; fi; cat "$STATE/cluster.token" ;;
    enable)
      need openssl; local token; token="$(mesh_cmd token)"; echo "✅ Mesh token created"; echo "Restart the controller on your LAN with:"; echo "NXYZ_LISTEN=0.0.0.0:8080 NXYZ_CLUSTER_TOKEN=$token make local-up"; echo; echo "On another node run the host agent with NXYZ_CONTROL_PLANE=http://<THIS-MAC-LAN-IP>:8080 and the same token." ;;
    status) ensure_cloud; api GET /api/v1/nodes; echo ;;
    *) echo "mesh: token | enable | status" ;;
  esac
}

tools_status() {
  echo "NXYZ FREE TOOL PLANE"
  echo "  storage    ✅ local object buckets"
  echo "  deploy     ✅ OCI image + Git build flow"
  echo "  db         ✅ rootless PostgreSQL"
  echo "  monitor    ✅ cloud/container metrics"
  echo "  logs       ✅ platform/workload logs"
  echo "  backup     ✅ files + PostgreSQL dumps"
  echo "  secrets    ✅ encrypted local secrets"
  echo "  registry   ✅ private OCI registry"
  echo "  git        ✅ private bare Git repos"
  echo "  ai         ✅ llama.cpp local API hook"
  echo "  agents     ✅ agent workload launcher"
  echo "  dns        ✅ NXYZ service discovery records"
  echo "  terminal   ✅ workload shell"
  echo "  catalog    ✅ one-command app templates"
  echo "  proxy      ✅ named route registry"
  echo "  mesh       ✅ multi-node join/token flow"
}

cmd="${1:-tools}"; shift || true
case "$cmd" in
  tools) tools_status ;;
  storage) storage_cmd "$@" ;;
  deploy) deploy_cmd "$@" ;;
  db|database) db_cmd "$@" ;;
  monitor) monitor_cmd "$@" ;;
  logs) logs_cmd "$@" ;;
  backup) backup_cmd "$@" ;;
  secrets|secret) secrets_cmd "$@" ;;
  registry) registry_cmd "$@" ;;
  git) git_cmd "$@" ;;
  ai) ai_cmd "$@" ;;
  agents|agent) agents_cmd "$@" ;;
  dns) dns_cmd "$@" ;;
  terminal|term) terminal_cmd "$@" ;;
  catalog|apps) catalog_cmd "$@" ;;
  proxy) proxy_cmd "$@" ;;
  mesh) mesh_cmd "$@" ;;
  *) fail "Unknown NXYZ tool: $cmd" ;;
esac
