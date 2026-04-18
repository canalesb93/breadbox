-include .local.env
export

TAILWIND_BIN := ./tailwindcss-extra

.PHONY: dev dev-watch dev-stop build test test-integration lint generate migrate-up migrate-down migrate-create sqlc sqlc-install seed db db-stop docker-up docker-down css css-watch css-install air-install templ templ-install templ-check

PORT ?= 8080

# generate ensures gitignored build artifacts exist.
# Skips if artifacts are already present (e.g., copied by .worktreeinclude).
# Run 'make sqlc', 'make templ', or 'make css' directly to force regeneration.
generate: templ
	@if [ ! -f internal/db/models.go ]; then $(MAKE) sqlc; fi
	@if [ ! -f static/css/styles.css ]; then $(MAKE) css; fi

# templ-install pulls the templ CLI if it's missing. Pinned via go.mod so the
# version the CLI understands always matches the runtime lib the binary
# compiles against.
templ-install:
	@if ! command -v templ &>/dev/null; then \
		echo "Installing templ..."; \
		go install github.com/a-h/templ/cmd/templ@latest; \
	fi

# templ generates *_templ.go for every *.templ file. Always runs — templ is
# fast enough (a few hundred ms) that the cost beats the complexity of
# timestamp-based skip logic, and missing generated files break the build.
templ: templ-install
	templ generate

# templ-check fails if generated files are stale. Used in CI so a PR cannot
# land with hand-edited .templ files that nobody regenerated.
templ-check: templ-install
	templ generate --ignore-always-generated
	@git diff --exit-code --name-only -- '*_templ.go' >/dev/null 2>&1 || { \
		echo "::error::templ generate produced changes — run 'make templ' and commit"; \
		git --no-pager diff --name-only -- '*_templ.go'; \
		exit 1; \
	}

dev: generate
	@if [ -z "$$DATABASE_URL" ]; then \
		echo "Error: DATABASE_URL is not set."; \
		echo "  Set it for local dev:  export DATABASE_URL=postgres://breadbox:breadbox@localhost:5432/breadbox?sslmode=disable"; \
		echo "  Or add it to .local.env (auto-loaded by Make)"; \
		exit 1; \
	fi
	@if lsof -ti:$(PORT) >/dev/null 2>&1; then \
		echo "Error: port $(PORT) is already in use."; \
		echo "  - Run on another port:  make dev PORT=8081"; \
		echo "  - Or kill the existing process:  kill $$(lsof -ti:$(PORT))"; \
		exit 1; \
	fi
	@echo $(PORT) > .breadbox-port
	SERVER_PORT=$(PORT) go run ./cmd/breadbox serve; rm -f .breadbox-port

# dev-watch: hot-reload everything — Go rebuilds via air, CSS rebuilds via
# tailwind --watch, and BREADBOX_DEV_RELOAD=1 makes the running binary read
# templates + static files from disk so HTML/CSS edits apply without a restart.
# Only Go changes trigger a rebuild.
dev-watch: generate air-install
	@if [ -z "$$DATABASE_URL" ]; then \
		echo "Error: DATABASE_URL is not set."; \
		echo "  Set it for local dev:  export DATABASE_URL=postgres://breadbox:breadbox@localhost:5432/breadbox?sslmode=disable"; \
		echo "  Or add it to .local.env (auto-loaded by Make)"; \
		exit 1; \
	fi
	@if lsof -ti:$(PORT) >/dev/null 2>&1; then \
		echo "Error: port $(PORT) is already in use."; \
		echo "  - Run on another port:  make dev-watch PORT=8081"; \
		echo "  - Or kill the existing process:  kill $$(lsof -ti:$(PORT))"; \
		exit 1; \
	fi
	@echo $(PORT) > .breadbox-port
	@trap 'rm -f .breadbox-port; kill 0' EXIT INT TERM; \
		$(TAILWIND_BIN) -i input.css -o static/css/styles.css --watch & \
		BREADBOX_DEV_RELOAD=1 SERVER_PORT=$(PORT) air -c .air.toml; \
		wait

air-install:
	@if ! command -v air &>/dev/null; then \
		echo "Installing air..."; \
		go install github.com/air-verse/air@latest; \
	fi

dev-stop:
	@pids=$$(lsof -ti:8080-8099 2>/dev/null | sort -u || true); \
	if [ -z "$$pids" ]; then \
		echo "No dev instances running."; \
	else \
		echo "$$pids" | xargs kill 2>/dev/null; \
		echo "Stopped dev instances on ports 8080-8099: $$pids"; \
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

sqlc-install:
	@if ! command -v sqlc &>/dev/null; then \
		echo "Installing sqlc..."; \
		go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0; \
	fi

sqlc: sqlc-install
	@rm -f internal/db/*.sql.go internal/db/models.go internal/db/db.go
	sqlc generate

seed:
	go run ./cmd/breadbox seed

db:
	docker compose up -d db
	@echo "Waiting for Postgres..."
	@until docker compose exec db pg_isready -U breadbox -q 2>/dev/null; do sleep 1; done
	@echo "Postgres ready on localhost:5432"

db-stop:
	docker compose stop db

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

css-install:
	@if [ ! -f $(TAILWIND_BIN) ]; then \
		echo "Downloading tailwindcss-extra..."; \
		curl -sLo $(TAILWIND_BIN) -m 120 https://github.com/dobicinaitis/tailwind-cli-extra/releases/latest/download/tailwindcss-extra-$$(uname -s | tr '[:upper:]' '[:lower:]')-$$(uname -m | sed 's/x86_64/x64/' | sed 's/aarch64/arm64/'); \
		chmod +x $(TAILWIND_BIN); \
	fi

css: css-install
	$(TAILWIND_BIN) -i input.css -o static/css/styles.css --minify

css-watch: css-install
	$(TAILWIND_BIN) -i input.css -o static/css/styles.css --watch
