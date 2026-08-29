-include .env
export

.PHONY: build run-api run-ingest run-extract migrate-up migrate-down seed test vet tidy docker-build docker-up docker-migrate docker-ingest docker-extract

build:
	go build ./...

run-api:
	go run ./cmd/api

run-ingest:
	go run ./cmd/ingest

run-extract:
	go run ./cmd/extract

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

seed:
	docker compose exec -T db psql -U $(POSTGRES_USER) -d $(POSTGRES_DB) -f - < seed/seed.sql

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

docker-build:
	docker compose build

docker-up:
	docker compose up db api

docker-migrate:
	docker compose run --rm migrate

docker-ingest:
	docker compose run --rm ingest

docker-extract:
	docker compose run --rm extract
