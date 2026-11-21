.PHONY: hooks verify dev run web build seed test lint e2e icons docker

IMAGE ?= ghcr.io/agusespa/loom:latest
PLATFORMS ?= linux/amd64,linux/arm64

hooks:
	git config core.hooksPath scripts

verify:
	./scripts/verify.sh

# Go API on :8080 plus the Vite dev server on :5173 (proxies /api to Go).
dev:
	@trap 'kill 0' INT TERM EXIT; \
	go run ./cmd/loom & \
	npm --prefix web run dev -- --port 5173

# Build the frontend and run the production server on :8080.
run: web
	go run ./cmd/loom

# Build the frontend and copy it where the Go binary embeds it from.
web:
	npm --prefix web ci
	npm --prefix web run build
	find internal/ui/dist -mindepth 1 ! -name .gitkeep -exec rm -rf {} +
	cp -R web/build/. internal/ui/dist/

build: web
	CGO_ENABLED=0 go build -o bin/loom ./cmd/loom

# Sample data for this week, written the way n8n would (local only).
seed:
	./scripts/seed.sh http://localhost:8080

test:
	go test -v -race ./...
	npm --prefix web test

lint:
	golangci-lint run ./...

# test:e2e builds the frontend first, so it always tests the current code.
e2e:
	npm ci
	npm --prefix web ci
	npx playwright install chromium
	npm run test:e2e

# Redraw the PNG icons in web/static.
icons:
	go run ./scripts/icons

docker:
	docker buildx build --platform $(PLATFORMS) -t $(IMAGE) .
