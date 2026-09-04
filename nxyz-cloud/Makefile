.PHONY: test build run up down smoke local-up local-down host-agent

test:
	go test ./...

build:
	go build -o bin/nxyz-controlplane ./cmd/controlplane
	go build -o bin/nxyz-agent ./cmd/agent

run:
	NXYZ_STATE=./.nxyz/state.json go run ./cmd/controlplane

# Docker Compose demo path (scheduler-only workers).
up:
	docker compose up --build -d

down:
	docker compose down

# Zero-license-cost executing path. Uses rootless Podman on the host and does
# not rely on shell executable bits being preserved by an archive/download.
local-up:
	bash ./scripts/start-local.sh

local-down:
	bash ./scripts/stop-local.sh

host-agent:
	bash ./scripts/run-host-agent.sh

smoke:
	sh ./scripts/smoke.sh
