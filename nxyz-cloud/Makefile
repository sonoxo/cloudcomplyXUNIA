.PHONY: test build run up down smoke

test:
	go test ./...

build:
	go build -o bin/nxyz-controlplane ./cmd/controlplane
	go build -o bin/nxyz-agent ./cmd/agent

run:
	NXYZ_STATE=./.nxyz/state.json go run ./cmd/controlplane

up:
	docker compose up --build -d

down:
	docker compose down

smoke:
	sh ./scripts/smoke.sh
