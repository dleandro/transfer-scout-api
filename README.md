# transfer-scout-api

Go backend for Transfer Scout — a Premier League transfer-rumour aggregator.
It ingests articles from PL news RSS feeds, extracts structured rumour data
with an LLM, clusters rumours into per-deal threads, and serves them over a
REST API.

See [CLAUDE.md](./CLAUDE.md) for architecture, decisions, and current status.

## Requirements

- Go 1.25+
- Docker (for local Postgres)

## Local development

```sh
cp .env.example .env

docker compose up -d db
make migrate-up
make seed

make run-api      # http://localhost:8080
make run-pipeline # ingest then extract, as the scheduled job does
make run-ingest   # or one stage at a time
make run-extract
```

## Makefile targets

| Target          | Description                                  |
|-----------------|-----------------------------------------------|
| `make build`    | `go build ./...`                              |
| `make run-api`  | run the REST API                              |
| `make run-pipeline` | run ingest then extract (the scheduled production job) |
| `make run-ingest` | run the RSS ingest poller once                |
| `make run-extract` | drain the unprocessed queue through the LLM extractor (stub without `EXTRACT_API_KEY`) |
| `make migrate-up` | apply all pending migrations                |
| `make migrate-down` | roll back the last migration              |
| `make seed`     | load `seed/seed.sql` (clubs + sources) into the DB running in Docker |
| `make test`     | `go test ./...`                               |
| `make vet`      | `go vet ./...`                                |
| `make tidy`     | `go mod tidy`                                 |