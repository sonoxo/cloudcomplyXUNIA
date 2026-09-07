#!/usr/bin/env bash
set -eo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CFG_DIR="$HOME/.config/nxyz"
UNIT_DIR="$HOME/.config/systemd/user"
UNIT="$UNIT_DIR/nxyz-cloud.service"
fail(){ echo "❌ $*" >&2; exit 1; }
need(){ command -v "$1" >/dev/null 2>&1 || fail "Required command not found: $1"; }

install_server(){
  [[ "$(uname -s)" == "Linux" ]] || fail "Dedicated server install currently targets Linux (Ubuntu/Debian recommended)"
  need systemctl; need podman; need go; need curl; need openssl
  local data="${1:-$HOME/.local/share/nxyz-cloud}"
  mkdir -p "$data" "$data/podman" "$CFG_DIR" "$UNIT_DIR"
  data="$(cd "$data" && pwd)"
  printf '%s\n' "$data" >"$CFG_DIR/data-dir"
  chmod 700 "$data" "$CFG_DIR" 2>/dev/null || true
  if [[ ! -f "$data/cluster.token" ]]; then umask 077; openssl rand -hex 32 >"$data/cluster.token"; fi
  touch "$data/mesh.enabled"
  cat >"$data/podman-storage.conf" <<EOF
[storage]
driver = "overlay"
graphroot = "$data/podman"
runroot = "/run/user/$(id -u)/containers"
EOF
  cat >"$UNIT" <<EOF
[Unit]
Description=NXYZ Cloud private application server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=NXYZ_LOCAL_STATE_DIR=$data
Environment=CONTAINERS_STORAGE_CONF=$data/podman-storage.conf
ExecStart=/bin/bash $ROOT/scripts/run-server.sh
ExecStop=/bin/bash $ROOT/scripts/stop-local.sh
Restart=always
RestartSec=5
TimeoutStopSec=20

[Install]
WantedBy=default.target
EOF
  systemctl --user daemon-reload
  systemctl --user enable --now nxyz-cloud.service
  if command -v loginctl >/dev/null 2>&1; then
    loginctl enable-linger "$USER" >/dev/null 2>&1 || echo "ℹ️ Run 'sudo loginctl enable-linger $USER' once if you want NXYZ to start before login."
  fi
  echo "✅ NXYZ dedicated server installed"
  echo "   Data root: $data"
  echo "   Podman:    $data/podman"
  echo "   Service:   nxyz-cloud.service"
  echo "   Mesh:      enabled"
  echo "   Token:     nxyz mesh token"
}

status_server(){
  if [[ -f "$CFG_DIR/data-dir" ]]; then echo "Data root: $(cat "$CFG_DIR/data-dir")"; else echo "Data root: not configured"; fi
  if command -v systemctl >/dev/null 2>&1; then systemctl --user --no-pager --full status nxyz-cloud.service || true; fi
}

uninstall_server(){
  if command -v systemctl >/dev/null 2>&1; then systemctl --user disable --now nxyz-cloud.service >/dev/null 2>&1 || true; fi
  rm -f "$UNIT" "$CFG_DIR/data-dir"
  if command -v systemctl >/dev/null 2>&1; then systemctl --user daemon-reload || true; fi
  echo "✅ NXYZ server service removed. Persistent data was left untouched."
}

case "${1:-status}" in
  install) shift; install_server "${1:-}" ;;
  status) status_server ;;
  restart) systemctl --user restart nxyz-cloud.service; status_server ;;
  logs) journalctl --user -u nxyz-cloud.service -n "${2:-100}" --no-pager ;;
  uninstall) uninstall_server ;;
  *) echo "server: install [DATA_DIR] | status | restart | logs [N] | uninstall" ;;
esac
