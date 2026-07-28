-include .env
export

.PHONY: build run-api run-ingest run-extract migrate-up migrate-down seed test vet tidy

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
