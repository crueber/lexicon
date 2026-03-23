.PHONY: build build-frontend embed-frontend run run-frontend test lint clean sqlc-generate docker-build migrate-up migrate-down

BINARY := lexicon
WEB_DIR := web
DIST_DIR := $(WEB_DIR)/dist
EMBED_DIR := internal/server/dist

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

# Apply all pending migrations.
migrate-up:
	@echo "migrate-up: not yet implemented (requires database module)"

# Roll back last migration.
migrate-down:
	@echo "migrate-down: not yet implemented (requires database module)"

# Remove build artifacts.
clean:
	rm -f $(BINARY)
	rm -rf $(DIST_DIR)
	rm -rf $(EMBED_DIR)
