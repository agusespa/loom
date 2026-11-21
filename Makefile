.PHONY: run seed build test lint icons docker

IMAGE ?= ghcr.io/agusespa/loom:latest
PLATFORMS ?= linux/amd64,linux/arm64

# Server on :8080 with a local database in data/. Frontend edits need a restart (they are embedded).
run:
	go run ./cmd/loom

# Sample data for this week, written the way n8n would (local only).
seed:
	./scripts/seed.sh http://localhost:8080

build:
	CGO_ENABLED=0 go build -o bin/loom ./cmd/loom

test:
	go test -race -coverprofile=coverage.out ./...
	node --check internal/ui/static/app.js
	node --check internal/ui/static/sw.js

lint:
	gofmt -l . | (! grep .)
	go vet ./...

icons:
	go run ./scripts/icons

docker:
	docker buildx build --platform $(PLATFORMS) -t $(IMAGE) .
