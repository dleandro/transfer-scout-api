# transfer-scout-api

Go backend for Transfer Scout — a football transfer-rumour aggregator.
It ingests articles from news RSS feeds, extracts structured rumour data
with an LLM, clusters rumours into per-deal threads, and serves them over a
REST API. Clubs are matched against a roster of leagues kept as one JSON
file per league in `internal/store/roster/`.

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
seeded sources, which migrations don't create:

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
| `make migrate-up` | apply all pending migrations, then sync the league/club roster |
| `make migrate-down` | roll back the last migration              |
| `make seed`     | load `seed/seed.sql` (roster leagues + clubs, sources) into the DB running in Docker |
| `make test`     | `go test ./...`                               |
| `make vet`      | `go vet ./...`                                |
| `make tidy`     | `go mod tidy`                                 |

## Club roster

Each file in `internal/store/roster/` is one league: its `name`, `short_name`
and `clubs`, each with a canonical `name`, `short_name`, `crest_url`
(an en.wikipedia/upload.wikimedia.org file URL, or `""`) and optional
`aliases`. To add a league, add a file; for promotion and relegation, edit the
club lists. Then run `go test ./internal/store/`: `TestSeedSQLMatchesRoster`
prints the block to paste into `seed/seed.sql`. `make migrate-up` (and the
production `transfer-scout-migrate` job) applies the roster with
`store.SyncRoster`, which is safe to re-run.

## Extraction environment

The extract stage (`cmd/pipeline`, or `cmd/extract` alone) asks Jev (TypeSafe System One) whether each article is a
transfer rumour before paying Claude to extract it. See `.env.example`.

| Variable | Default | Meaning |
|----------|---------|---------|
| `EXTRACT_API_KEY` | empty | Anthropic key. Empty runs the stub extractor (nothing extracted). |
| `EXTRACT_MODEL` | `claude-haiku-4-5-20251001` | Claude model that extracts player, clubs, fee and status. |
| `TYPESAFE_API_KEY` | empty | Jev key. Required whenever `EXTRACT_API_KEY` is set; the job refuses to start without it rather than call Claude ungated. |
| `JEV_MODEL` | `jev-1.13.0` | Pinned Jev version. Re-measure `JEV_MIN_PROBABILITY` when changing it. |
| `JEV_MIN_PROBABILITY` | `0.5` | Jev probability an article needs to reach Claude. The only rumour/not-rumour decision. Placeholder, unmeasured — choose it from the eval's threshold sweep. |
| `EXTRACT_BASE_URL`, `TYPESAFE_BASE_URL` | empty | Endpoint overrides for local smoke runs against fake servers. |

`go run ./cmd/reclassify` (dry run) / `-apply` re-checks rumours stored under
the old gate with Jev alone and deletes those whose articles all fall below
`JEV_MIN_PROBABILITY`. Needs `DATABASE_URL` and `TYPESAFE_API_KEY`.
