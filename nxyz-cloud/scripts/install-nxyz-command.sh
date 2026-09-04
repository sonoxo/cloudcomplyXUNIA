#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

choose_bin_dir() {
  local candidate
  for candidate in /opt/homebrew/bin /usr/local/bin "$HOME/.local/bin"; do
    if [[ -d "$candidate" && -w "$candidate" ]]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done

  mkdir -p "$HOME/.local/bin"
  printf '%s\n' "$HOME/.local/bin"
}

BIN_DIR="$(choose_bin_dir)"
TARGET="$BIN_DIR/nxyz"

cat > "$TARGET" <<EOF
#!/usr/bin/env bash
set -euo pipefail

NXYZ_DIR='$ROOT'
CONTROL_PLANE="\${NXYZ_CONTROL_PLANE:-http://127.0.0.1:8080}"

cd "\$NXYZ_DIR"

cmd="\${1:-start}"
case "\$cmd" in
  start)
    echo "☁️  Starting NXYZ Cloud..."
    git pull --ff-only
    make local-up
    ;;
  stop)
    make local-down
    ;;
  restart)
    make local-down || true
    make local-up
    ;;
  status)
    echo "☁️  NXYZ Cloud status"
    if curl -fsS "\$CONTROL_PLANE/healthz" >/dev/null 2>&1; then
      echo "✅ Control plane: online"
      curl -fsS "\$CONTROL_PLANE/api/v1/nodes"
      echo
    else
      echo "❌ Control plane: offline"
      exit 1
    fi
    ;;
  dashboard|open)
    if [[ "\$(uname -s)" == "Darwin" ]]; then
      open "\$CONTROL_PLANE/"
    else
      echo "\$CONTROL_PLANE/"
    fi
    ;;
  update)
    git pull --ff-only
    ;;
  logs)
    echo "--- control plane ---"
    tail -n 40 .nxyz/controlplane.log 2>/dev/null || true
    echo "--- agent ---"
    tail -n 40 .nxyz/agent.log 2>/dev/null || true
    ;;
  path)
    printf '%s\n' "\$NXYZ_DIR"
    ;;
  shell)
    cd "\$NXYZ_DIR"
    exec "\${SHELL:-/bin/zsh}" -l
    ;;
  help|-h|--help)
    cat <<'HELP'
NXYZ Cloud CLI

  nxyz             Start/update NXYZ Cloud and open the dashboard
  nxyz status      Show cloud health and nodes
  nxyz dashboard   Open the dashboard
  nxyz restart     Restart local NXYZ services
  nxyz stop        Stop local NXYZ services
  nxyz update      Pull the latest NXYZ code
  nxyz logs        Show recent control-plane and agent logs
  nxyz path        Print the NXYZ Cloud directory
  nxyz shell       Enter a shell inside the NXYZ Cloud directory
HELP
    ;;
  *)
    echo "Unknown command: \$cmd" >&2
    echo "Run: nxyz help" >&2
    exit 2
    ;;
esac
EOF

chmod 0755 "$TARGET"

echo "✅ Installed NXYZ command: $TARGET"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *)
    echo
    echo "⚠️  $BIN_DIR is not currently in PATH."
    echo "Add this to ~/.zprofile (not ~/.zshrc):"
    echo "export PATH=\"$BIN_DIR:\$PATH\""
    ;;
esac

echo
echo "Run: nxyz"
