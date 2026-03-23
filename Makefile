.PHONY: build build-frontend embed-frontend run run-frontend dev test lint clean sqlc-generate docker-build migrate-up migrate-down

BINARY := lexicon
WEB_DIR := web
DIST_DIR := $(WEB_DIR)/dist
EMBED_DIR := internal/server/dist
DATA_DIR ?= ./data

# Build Go binary with embedded frontend.
build: embed-frontend
	CGO_ENABLED=0 go build -o $(BINARY) ./cmd/lexicon

# Build frontend only.
build-frontend:
	cd $(WEB_DIR) && npm install && npm run build

# Build frontend and copy into internal/server for embedding.
embed-frontend: build-frontend
	rm -rf $(EMBED_DIR)
	cp -r $(DIST_DIR) $(EMBED_DIR)

# Run in development mode.
run:
	DEV_MODE=true go run -tags dev ./cmd/lexicon

# Run full dev environment (instructions).
dev:
	@echo "Development requires two terminals:"
	@echo "  Terminal 1: make run           (Go backend on :6060)"
	@echo "  Terminal 2: make run-frontend   (Vite dev server on :5173)"
	@echo ""
	@echo "Or for a quick test with embedded frontend:"
	@echo "  make build && ./lexicon"

# Run Vite dev server.
run-frontend:
	cd $(WEB_DIR) && npm run dev

# Run all tests with race detector.
test:
	go test -tags dev -race ./...

# Run static analysis.
lint:
	go vet -tags dev ./...

# Regenerate sqlc code.
sqlc-generate:
	sqlc generate

# Build Docker image.
docker-build:
	docker build -t lexicon .

# Apply all pending migrations (uses the Go binary).
migrate-up:
	@echo "Migrations run automatically on server startup."
	@echo "To run manually, start the server: make run"

# Roll back last migration (requires golang-migrate CLI).
migrate-down:
	@command -v migrate >/dev/null 2>&1 || { echo "Install golang-migrate CLI: go install -tags 'sqlite' github.com/golang-migrate/migrate/v4/cmd/migrate@latest"; exit 1; }
	migrate -source "file://migrations" -database "sqlite://$(DATA_DIR)/lexicon.db" down 1

# Remove build artifacts.
clean:
	rm -f $(BINARY)
	rm -rf $(DIST_DIR)
	rm -rf $(EMBED_DIR)
