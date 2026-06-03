-include .local.env
export

TAILWIND_BIN := ./tailwindcss-extra

.PHONY: dev dev-watch dev-stop dev-stop-all dev-bg dev-ps dev-reap dev-shot build build-headless build-lite test test-integration lint lint-headless lint-lite generate migrate-up migrate-down migrate-create sqlc sqlc-install sqlc-tag seed db db-stop docker-up docker-down css css-watch css-install air-install templ templ-install templ-check openapi-validate agent-sidecar agent-sidecar-install agent-sidecar-install-user agent-sidecar-typecheck

PORT ?= 8080

# generate ensures gitignored build artifacts exist and are up to date.
# - sqlc: only rebuilds if the generated models file is missing (queries are
#   regenerated out-of-band via `make sqlc` when queries change).
# - css: rebuilds whenever styles.css is missing OR older than input.css, so
#   editing input.css and running `make dev` picks up the change without a
#   manual `make css`. Previously this used `-f` existence only, which caused
#   plain `make dev` to serve stale embedded CSS whenever input.css had been
#   edited since the last full build.
# Run 'make sqlc', 'make templ', or 'make css' directly to force regeneration.
generate: templ
	@if [ ! -f internal/db/models.go ]; then $(MAKE) sqlc; fi
	@if [ ! -f static/css/styles.css ] || [ input.css -nt static/css/styles.css ]; then $(MAKE) css; fi

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
#
# templ does not currently emit `//go:build` constraints, so we prepend
# `//go:build !headless && !lite` to every generated `*_templ.go` file
# under internal/templates/components — those files belong to the
# dashboard surface and must be excluded from headless and lite builds.
templ: templ-install
	templ generate
	@find internal/templates/components -name '*_templ.go' -print0 | while IFS= read -r -d '' f; do \
		if ! head -1 "$$f" | grep -q '^//go:build'; then \
			printf '//go:build !headless && !lite\n\n' | cat - "$$f" > "$$f.tmp" && mv "$$f.tmp" "$$f"; \
		fi; \
	done

# templ-check fails if generated files are stale. Used in CI so a PR cannot
# land with hand-edited .templ files that nobody regenerated.
templ-check: templ-install
	templ generate --ignore-always-generated
	@git diff --exit-code --name-only -- '*_templ.go' >/dev/null 2>&1 || { \
		echo "::error::templ generate produced changes — run 'make templ' and commit"; \
		git --no-pager diff --name-only -- '*_templ.go'; \
		exit 1; \
	}

# `make dev` runs the prod-like embedded-FS server. We rebuild CSS up front
# so any template changes (new utility classes, new templ files) are reflected
# in the bundled styles.css before the binary boots — otherwise Tailwind's
# scanned class list goes stale and the page renders without the classes
# this dev session just added. `dev-watch` doesn't need this because
# tailwindcss-extra --watch is running alongside air.
dev: generate css
	@if [ -z "$$DATABASE_URL" ]; then \
		echo "Error: DATABASE_URL is not set."; \
		echo "  Set it for local dev:  export DATABASE_URL=postgres://breadbox:breadbox@localhost:5432/breadbox?sslmode=disable"; \
		echo "  Or add it to .local.env (auto-loaded by Make)"; \
		exit 1; \
	fi
	@port=$$(BB_PORT_EXPLICIT='$(if $(filter command line,$(origin PORT)),1,)' PORT='$(PORT)' ./scripts/dev-port) || exit 1; \
	if lsof -ti:$$port >/dev/null 2>&1; then \
		echo "Error: port $$port is already in use."; \
		echo "  - Run on another port:  make dev PORT=8085"; \
		echo "  - Or kill the existing process:  kill $$(lsof -ti:$$port)"; \
		exit 1; \
	fi; \
	echo $$port > .breadbox-port; \
	echo "==> dev server: http://localhost:$$port"; \
	SERVER_PORT=$$port go run ./cmd/breadbox serve; rm -f .breadbox-port

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
	@port=$$(BB_PORT_EXPLICIT='$(if $(filter command line,$(origin PORT)),1,)' PORT='$(PORT)' ./scripts/dev-port) || exit 1; \
	if lsof -ti:$$port >/dev/null 2>&1; then \
		echo "Error: port $$port is already in use."; \
		echo "  - Run on another port:  make dev-watch PORT=8085"; \
		echo "  - Or kill the existing process:  kill $$(lsof -ti:$$port)"; \
		exit 1; \
	fi; \
	echo $$port > .breadbox-port; \
	echo "==> dev-watch server: http://localhost:$$port"; \
	trap 'rm -f .breadbox-port; kill 0' EXIT INT TERM; \
		$(TAILWIND_BIN) -i input.css -o static/css/styles.css --watch & \
		BREADBOX_DEV_RELOAD=1 SERVER_PORT=$$port air -c .air.toml; \
		wait

air-install:
	@if ! command -v air &>/dev/null; then \
		echo "Installing air..."; \
		go install github.com/air-verse/air@latest; \
	fi

# dev-bg: start (or reuse) a managed background dev server for THIS worktree.
# Idempotent — reuses a healthy server if one is already up. Prints the URL.
# This is the validation/pre-warm entrypoint: no port fishing, tracked for
# cleanup, and reaped automatically when the session ends or the worktree is
# removed. For interactive hot-reload instead, use `make dev-watch`.
dev-bg:
	@./scripts/dev-server ensure

# dev-shot: rebuild, (re)start the managed server, and screenshot routes.
# Pass routes + flags via ARGS, e.g.:
#   make dev-shot ARGS="/transactions --mobile --wait '.join button'"
# Prints the saved JPEG paths (upload via the github-image-hosting skill).
dev-shot:
	@./scripts/ui-validate $(ARGS)

# dev-stop: stop ONLY this worktree's managed server (safe to run while other
# worktrees keep theirs). Use dev-stop-all for the blunt range kill.
dev-stop:
	@./scripts/dev-server stop
	@echo "Stopped this worktree's managed dev server (if any)."
	@echo "To stop every breadbox on 8080-8099, run: make dev-stop-all"

# dev-ps: list every managed dev server (port, pid, branch, worktree, alive).
dev-ps:
	@./scripts/dev-server ps

# dev-reap: kill orphaned servers — dead pids and servers whose worktree was
# removed. Leaves healthy, actively-owned servers alone.
dev-reap:
	@./scripts/dev-server reap
	@echo "Reaped orphaned dev servers."

# dev-stop-all: blunt instrument — kill every process listening on 8080-8099,
# including OTHER active worktrees. SIGTERM then SIGKILL survivors, then clears
# the registry of entries whose process is actually gone (a survivor stays
# tracked so dev-ps/dev-reap can still find it).
dev-stop-all:
	@./scripts/dev-server stop-all

build: generate
	go build -o breadbox ./cmd/breadbox

# build-headless: server + REST + MCP + OAuth + webhooks, NO dashboard assets.
# Strips internal/admin and internal/templates from the binary via
# -tags=headless. See .claude/rules/build-tags.md.
build-headless: generate
	go build -tags=headless -o breadbox ./cmd/breadbox

# build-lite: CLI-only binary (named `breadbox-cli`). No server packages,
# no DB drivers, no provider SDKs — for remote agents that only need to
# drive a Breadbox over HTTP.
build-lite:
	go build -tags=lite -o breadbox-cli ./cmd/breadbox

# agent-sidecar: build the standalone breadbox-agent binary that the Go
# server exec's per Claude Agent SDK run. Output is bin/breadbox-agent —
# a self-contained binary with the Bun runtime embedded (~50 MB).
# The Go server discovers this binary via app_config agent.runtime_path,
# the BREADBOX_AGENT_BIN env var, ./bin/breadbox-agent (cwd-relative),
# ~/.breadbox/agent-bin/breadbox-agent (per-user — see -install-user
# target below), then $PATH. Local dev: build once with -install-user and
# every worktree picks it up; production: the Docker image bakes it in.
agent-sidecar-install:
	@if ! command -v bun &>/dev/null; then \
		echo "Error: bun is not installed. Install via 'curl -fsSL https://bun.sh/install | bash'."; \
		exit 1; \
	fi
	cd agent/sidecar && bun install --frozen-lockfile || cd agent/sidecar && bun install

agent-sidecar: agent-sidecar-install
	cd agent/sidecar && bun run build
	@mkdir -p bin
	@cp agent/sidecar/bin/breadbox-agent bin/breadbox-agent 2>/dev/null || true
	@echo "Built bin/breadbox-agent (also at agent/sidecar/bin/breadbox-agent)."

# agent-sidecar-install-user: build the sidecar (if not already built) and
# copy it to ~/.breadbox/agent-bin/breadbox-agent — the per-user slot in
# LocateBinary's discovery chain (priority 4). Run this ONCE on a fresh
# machine; from then on every worktree's `breadbox serve` finds it without
# having to bake a per-worktree `bin/breadbox-agent`. This is the canonical
# local-dev install path; the worktree session-start hook checks for this
# file and hints to run this target when missing.
agent-sidecar-install-user: agent-sidecar
	@mkdir -p $$HOME/.breadbox/agent-bin
	@cp bin/breadbox-agent $$HOME/.breadbox/agent-bin/breadbox-agent
	@chmod +x $$HOME/.breadbox/agent-bin/breadbox-agent
	@echo "Installed $$HOME/.breadbox/agent-bin/breadbox-agent — discoverable by every worktree."

agent-sidecar-typecheck: agent-sidecar-install
	cd agent/sidecar && bun run typecheck

test: generate
	go test ./...

test-integration: generate
	DATABASE_URL=postgres://breadbox:breadbox@localhost:5432/breadbox_test?sslmode=disable go test -tags integration -count=1 -p 1 ./...

lint: generate
	go vet ./...

# lint-headless and lint-lite mirror the CI matrix locally. Run them before
# pushing a build-tag-touching change so you catch the per-tag breakage
# before the PR opens.
lint-headless: generate
	go vet -tags=headless ./...

lint-lite: generate
	go vet -tags=lite ./...

# openapi-validate lints the hand-authored openapi.yaml at the repo root.
# Real CI enforcement is the TestOpenAPIDrift integration test in
# internal/api — this target exists for local dev convenience and uses
# swagger-cli (Node) when available because there is no Go-native validator
# we want to add as a direct dependency.
openapi-validate:
	@command -v swagger-cli >/dev/null 2>&1 || { \
		echo "swagger-cli not found. Install with: npm install -g @apidevtools/swagger-cli"; \
		exit 1; \
	}
	swagger-cli validate openapi.yaml

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
	@$(MAKE) sqlc-tag

# sqlc-tag prepends `//go:build !lite` to every sqlc-generated file in
# internal/db. sqlc 1.23 (pinned in sqlc-install) does not yet support the
# `emit_build_tag` option (added in 1.27+), so we patch the artifacts in a
# follow-up step. CI runs the same target.
sqlc-tag:
	@for f in internal/db/db.go internal/db/models.go internal/db/*.sql.go; do \
		if [ -f "$$f" ] && ! head -1 "$$f" | grep -q '^//go:build'; then \
			printf '//go:build !lite\n\n' | cat - "$$f" > "$$f.tmp" && mv "$$f.tmp" "$$f"; \
		fi; \
	done

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
