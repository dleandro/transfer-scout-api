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

## Integration tests

`go test ./...` skips the Postgres-backed integration tests unless
`DATABASE_URL` is set. To run them, point `DATABASE_URL` at a database with
migrations applied **and** `seed/seed.sql` loaded — several tests rely on the
seeded sources and clubs, which migrations don't create:

```sh
docker compose up -d db
make migrate-up
make seed
make test
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
## Extraction environment

The extract stage (`cmd/pipeline`, or `cmd/extract` alone) asks Jev (TypeSafe System One) whether each article is a
transfer rumour before paying Claude to extract it. See `.env.example`.

| Variable | Default | Meaning |
|----------|---------|---------|
| `EXTRACT_API_KEY` | empty | Anthropic key. Empty runs the stub extractor (nothing extracted). |
| `EXTRACT_MODEL` | `claude-haiku-4-5-20251001` | Claude model that extracts player, clubs, fee and status. |
| `EXTRACT_MIN_CONFIDENCE` | `0.4` | Floor on Claude's field confidence. Provisional, unmeasured. |
| `TYPESAFE_API_KEY` | empty | Jev key. Required whenever `EXTRACT_API_KEY` is set; the job refuses to start without it rather than call Claude ungated. |
| `JEV_MODEL` | `jev-1.13.0` | Pinned Jev version. Re-measure `JEV_MIN_PROBABILITY` when changing it. |
| `JEV_MIN_PROBABILITY` | `0.5` | Jev probability an article needs to reach Claude. Placeholder, unmeasured — choose it from the eval's threshold sweep. |
| `EXTRACT_BASE_URL`, `TYPESAFE_BASE_URL` | empty | Endpoint overrides for local smoke runs against fake servers. |
