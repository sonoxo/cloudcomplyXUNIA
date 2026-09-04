#!/usr/bin/env bash
set -euo pipefail

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
    if ! podman machine inspect >/dev/null 2>&1; then
      echo "Initializing the local Podman machine..."
      podman machine init
    fi
    echo "Starting the local Podman machine..."
    podman machine start >/dev/null
  fi
fi

export NXYZ_EXECUTION_ENABLED="${NXYZ_EXECUTION_ENABLED:-true}"
export NXYZ_RUNTIME="$runtime"

echo "Starting NXYZ rootless execution agent with runtime: $runtime"
exec go run ./cmd/agent
