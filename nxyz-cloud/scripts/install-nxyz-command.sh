#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

choose_bin_dir() {
  local candidate
  for candidate in /opt/homebrew/bin /usr/local/bin "$HOME/.local/bin"; do
    if [[ -d "$candidate" && -w "$candidate" ]]; then printf '%s\n' "$candidate"; return 0; fi
  done
  mkdir -p "$HOME/.local/bin"; printf '%s\n' "$HOME/.local/bin"
}

BIN_DIR="$(choose_bin_dir)"
TARGET="$BIN_DIR/nxyz"

cat > "$TARGET" <<EOF
#!/usr/bin/env bash
set -eo pipefail

NXYZ_DIR='$ROOT'
if [[ -z "\${NXYZ_LOCAL_STATE_DIR:-}" && -f "\$HOME/.config/nxyz/data-dir" ]]; then
  export NXYZ_LOCAL_STATE_DIR="\$(cat "\$HOME/.config/nxyz/data-dir")"
fi
STATE_DIR="\${NXYZ_LOCAL_STATE_DIR:-\$NXYZ_DIR/.nxyz}"
if [[ -f "\$STATE_DIR/podman-storage.conf" ]]; then export CONTAINERS_STORAGE_CONF="\$STATE_DIR/podman-storage.conf"; fi
CONTROL_PLANE="\${NXYZ_CONTROL_PLANE:-}"
if [[ -z "\$CONTROL_PLANE" && -f "\$STATE_DIR/remote-control-plane" ]]; then CONTROL_PLANE="\$(cat "\$STATE_DIR/remote-control-plane")"; fi
CONTROL_PLANE="\${CONTROL_PLANE:-http://127.0.0.1:8080}"

cd "\$NXYZ_DIR"
cmd="\${1:-start}"
case "\$cmd" in
  start)
    echo "☁️  Starting NXYZ Cloud..."
    git pull --ff-only
    make local-up
    ;;
  stop) make local-down ;;
  restart) make local-down || true; make local-up ;;
  status) bash "\$NXYZ_DIR/scripts/nxyz-v1-tools.sh" info ;;
  dashboard|open)
    if [[ "\$(uname -s)" == "Darwin" ]]; then open "\$CONTROL_PLANE/"; else echo "\$CONTROL_PLANE/"; fi
    ;;
  update) git pull --ff-only ;;
  server)
    shift || true
    bash "\$NXYZ_DIR/scripts/nxyz-server.sh" "\$@"
    ;;
  logs|tools|storage|deploy|db|database|monitor|backup|secrets|secret|registry|git|ai|agents|agent|dns|terminal|term|catalog|apps|proxy|mesh|services|service|doctor|info)
    bash "\$NXYZ_DIR/scripts/nxyz-v1-tools.sh" "\$@"
    ;;
  path) printf '%s\n' "\$NXYZ_DIR" ;;
  data) printf '%s\n' "\$STATE_DIR" ;;
  shell) cd "\$NXYZ_DIR"; exec "\${SHELL:-/bin/zsh}" -l ;;
  help|-h|--help)
    cat <<'HELP'
NXYZ Cloud v1 CLI

Core
  nxyz                         Start/update NXYZ Cloud
  nxyz status                  Cloud summary + published services
  nxyz dashboard               Open the active controller dashboard
  nxyz doctor                  Verify runtime, controller and storage
  nxyz restart                 Restart local NXYZ services
  nxyz stop                    Stop local NXYZ services
  nxyz update                  Pull latest code
  nxyz logs [ID]               Platform logs or workload logs
  nxyz path                    Print NXYZ code directory
  nxyz data                    Print persistent data directory

Apps & services
  nxyz deploy image NAME IMAGE [CPU] [MEM] [PORT] [HEALTH]
  nxyz deploy git NAME URL [CPU] [MEM] [PORT] [HEALTH]
  nxyz services                List published app endpoints
  nxyz service open NAME       Open a published app directly
  nxyz service proxy NAME      Open through the NXYZ reverse proxy
  nxyz catalog install APP     Install an app template

Data & platform
  nxyz storage status          Total/free/NXYZ/Podman storage
  nxyz storage ...             Object/file buckets
  nxyz db ...                  PostgreSQL databases
  nxyz backup ...              Backup and restore
  nxyz secrets ...             Encrypted secrets
  nxyz registry ...            Private OCI registry
  nxyz git ...                 Private Git repositories
  nxyz ai ...                  llama.cpp local AI API
  nxyz agents ...              Agent workload launcher
  nxyz terminal ID             Shell into a workload
  nxyz monitor                 CPU/RAM/container metrics

Network & scale
  nxyz mesh enable             Make this PC the mesh controller
  nxyz mesh token              Show node-enrollment token
  nxyz mesh join URL TOKEN     Join another PC to the cloud
  nxyz mesh status             Show all cloud nodes
  nxyz mesh disable            Return to local-only mode

Dedicated Linux server / 1-TB node
  nxyz server install /mnt/nxyz   Install always-on systemd service
  nxyz server status              Show server status
  nxyz server logs 100            Show service logs
  nxyz server restart             Restart server
HELP
    ;;
  *) echo "Unknown command: \$cmd" >&2; echo "Run: nxyz help" >&2; exit 2 ;;
esac
EOF

chmod 0755 "$TARGET"
echo "✅ Installed NXYZ command: $TARGET"
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo; echo "⚠️  $BIN_DIR is not currently in PATH."; echo "Add this to ~/.zprofile (not ~/.zshrc):"; echo "export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac
echo; echo "Run: nxyz"
