.PHONY: dev build test lint migrate-up migrate-down migrate-create sqlc seed docker-up docker-down

dev:
	go run ./cmd/breadbox serve

build:
	go build -o breadbox ./cmd/breadbox

test:
	go test ./...

lint:
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
