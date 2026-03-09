-include .local.env
export

TAILWIND_BIN := ./tailwindcss-extra

.PHONY: dev build test test-integration lint generate migrate-up migrate-down migrate-create sqlc seed docker-up docker-down css css-watch css-install

# generate runs sqlc + css so all gitignored build artifacts exist
generate: sqlc css

dev: generate
	go run ./cmd/breadbox serve

build: generate
	go build -o breadbox ./cmd/breadbox

test: generate
	go test ./...

test-integration: generate
	DATABASE_URL=postgres://breadbox:breadbox@localhost:5432/breadbox_test?sslmode=disable go test -tags integration -count=1 ./...

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
