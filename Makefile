-include .local.env
export

TAILWIND_BIN := ./tailwindcss-extra

.PHONY: dev dev-stop build test test-integration lint generate migrate-up migrate-down migrate-create sqlc seed docker-up docker-down css css-watch css-install

PORT ?= 8080

# generate ensures gitignored build artifacts exist.
# Skips if artifacts are already present (e.g., copied by .worktreeinclude).
# Run 'make sqlc' or 'make css' directly to force regeneration.
generate:
	@if [ ! -f internal/db/models.go ]; then $(MAKE) sqlc; fi
	@if [ ! -f static/css/styles.css ]; then $(MAKE) css; fi

dev: generate
	@if lsof -ti:$(PORT) >/dev/null 2>&1; then \
		echo "Error: port $(PORT) is already in use."; \
		echo "  - Run on another port:  make dev PORT=8081"; \
		echo "  - Or kill the existing process:  kill $$(lsof -ti:$(PORT))"; \
		exit 1; \
	fi
	@echo $(PORT) > .breadbox-port
	SERVER_PORT=$(PORT) go run ./cmd/breadbox serve; rm -f .breadbox-port

dev-stop:
	@pids=$$(pgrep -f 'go run ./cmd/breadbox serve' 2>/dev/null || true); \
	if [ -z "$$pids" ]; then \
		echo "No dev instances running."; \
	else \
		echo "$$pids" | xargs kill 2>/dev/null; \
		echo "Stopped dev instances: $$pids"; \
	fi

build: generate
	go build -o breadbox ./cmd/breadbox

test: generate
	go test ./...

test-integration: generate
	DATABASE_URL=postgres://breadbox:breadbox@localhost:5432/breadbox_test?sslmode=disable go test -tags integration -count=1 -p 1 ./...

lint: generate
	go vet ./...

migrate-up:
	goose -dir internal/db/migrations postgres "$$DATABASE_URL" up

migrate-down:
	goose -dir internal/db/migrations postgres "$$DATABASE_URL" down

migrate-create:
	goose -dir internal/db/migrations create $(NAME) sql

sqlc:
	sqlc generate

seed:
	go run ./cmd/breadbox seed

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

css-install:
	@if [ ! -f $(TAILWIND_BIN) ]; then \
		echo "Downloading tailwindcss-extra..."; \
		curl -sLo $(TAILWIND_BIN) https://github.com/dobicinaitis/tailwind-cli-extra/releases/latest/download/tailwindcss-extra-$$(uname -s | tr '[:upper:]' '[:lower:]')-$$(uname -m | sed 's/x86_64/x64/' | sed 's/aarch64/arm64/'); \
		chmod +x $(TAILWIND_BIN); \
	fi

css: css-install
	$(TAILWIND_BIN) -i input.css -o static/css/styles.css --minify

css-watch: css-install
	$(TAILWIND_BIN) -i input.css -o static/css/styles.css --watch
