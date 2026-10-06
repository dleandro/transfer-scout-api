# CLAUDE.md

Context for future Claude Code sessions working on transfer-scout-api.

## What this is

Transfer Scout aggregates Premier League transfer rumours from many news
sources, deduplicates and clusters them into per-deal threads, scores source
reliability, and (later) adds a prediction game where users bet virtual
points on whether rumours come true. Revenue: display ads now, premium
subscriptions later.

Two repos under github.com/dleandro: `transfer-scout-api` (this repo, Go
backend, priority) and `transfer-scout-web` (Next.js frontend, later).

## Locked architecture decisions — do not re-litigate

- Stack: Go, Postgres, chi router, pgx v5, gofeed for RSS, golang-migrate.
- **Go version note**: the original brief specified Go 1.22, but the latest
  pgx/v5 (v5.10.0) requires Go ≥1.25 in its own go.mod, so `go.mod` here is
  pinned to `go 1.25.0` — matching the toolchain actually installed
  (1.25.3) rather than an artificially pinned older pgx. Flagged, not
  silently changed.
- Binaries: `cmd/api` (REST API) and `cmd/pipeline` (the scheduled job:
  ingest then extract in one process, one Cloud Run Job, one Cloud
  Scheduler trigger). The stage logic lives in `internal/pipeline`;
  `cmd/ingest` (RSS poller) and `cmd/extract` (LLM extraction worker,
  Anthropic Messages API, see Milestone 1.3) are thin binaries that run one
  stage each, for manual use. All are one-shot per invocation, not ticker
  loops — Cloud Scheduler owns cadence (production roadmap Task 5.1).
  Extract always runs after ingest whatever ingest did, since its input is
  the `processed = false` queue; a failed ingest still makes the process
  exit non-zero. Extract drains that queue in batches of 50 until it is
  empty or `EXTRACT_MAX_ARTICLES_PER_RUN` (default 500) articles have been
  attempted, and logs a warning when the cap leaves articles queued.
  `cmd/migrate` is a dev-only binary wrapping golang-migrate for
  `make migrate-up`/`make migrate-down` — not an application binary.
- Core entity is a "rumour": a long-lived thread UNIQUE per (player_id,
  to_club_id, transfer_window). The column is `transfer_window`, not
  `window` (reserved word).
- Each new article either opens a rumour or appends a `rumour_event` to its
  timeline. Ingestion and extraction are decoupled via `articles.processed`.
- Rumour status is roughly one-way: `rumoured → talks → advanced → medical →
  confirmed | collapsed`. Terminal states (`confirmed`, `collapsed`) will
  later resolve game predictions. Enforced in code via
  `models.RumourStatus.IsForwardTransition` / `IsTerminal`
  (`internal/models/models.go`) — a terminal status cannot transition
  further.
- LLM extraction contract lives in `internal/extract/extract.go` —
  `SystemPrompt` + `Result` struct. The model returns structured JSON per
  article (player, from/to club, status, fee range, summary, confidence).
- Extraction is gated: `extract.GatedExtractor` first asks Jev (TypeSafe
  System One, `internal/extract/jev.go`, a hand-rolled HTTP client — there
  is no Go SDK) a single Noul question restating `SystemPrompt`'s definition
  of a transfer rumour, with the article as a `{title, body}` state (body
  truncated to 24 KB). Below `JEV_MIN_PROBABILITY` the article is recorded
  as not a rumour and Claude is never called; at or above it, Claude
  extracts as before. Jev's probability and the versioned model id that
  answered are stored on the extraction JSON (`jev_probability`,
  `jev_model`) for later auditing. A Jev error fails the article — it never
  falls through to Claude; like every extraction failure,
  `pipeline.RunExtract` then marks it processed with a NULL extraction (not
  retried automatically). Both `cmd/pipeline` and `cmd/extract` get the
  gate from `pipeline.NewExtractor` → `extract.NewFromConfig`; with
  `EXTRACT_API_KEY` set and `TYPESAFE_API_KEY` empty, both refuse to
  start. `JEV_MODEL` is pinned to `jev-1.13.0`, and
  `JEV_MIN_PROBABILITY` (default 0.5) is an unmeasured placeholder: set it
  from the threshold sweep printed by `TestEval_JevGateClassifiesLabelledArticles`
  (`TYPESAFE_API_KEY=... go test ./internal/extract/ -run TestEval -v`).
- PL only for the MVP. Current window: `summer-2026` (`TRANSFER_WINDOW` env
  var, defaults to this in `internal/config`).

## Current status (as of Milestone 3.2)

**2026-08-06: Milestone 3.2 (comments)** — built on top of Milestone
3.1's `users`/auth stack. First user-generated-content table.

- New `comments` table (migration `0005`), `rumour_id`/`user_id` both
  `ON DELETE CASCADE`, a `CHECK (char_length(body) BETWEEN 1 AND 2000)`
  as defense-in-depth behind the handler's own validation.
- `internal/store/comments.go`: `CreateComment` (a single `INSERT ...
  RETURNING` CTE joined back to `users` for the author) and
  `ListComments` (same limit+1-and-trim `hasMore` pattern as
  `ListRumours`, oldest-first). **Real bug caught by the integration
  test, not by review**: the first draft aliased the CTE
  (`FROM inserted AS comments`) but then referenced the pre-alias name
  in the JOIN condition (`ON users.id = inserted.user_id`) — Postgres
  correctly rejects this once a FROM-clause entry is aliased. Fixed to
  reference `comments.user_id` throughout; this only surfaced once
  actually run against real Postgres, confirming these integration
  tests earn their keep.
- New `internal/store.ErrRumourNotFound` sentinel (detected via the
  insert's `23503` foreign-key-violation Postgres error code), mapped
  to `404` by the handler — same idea as `sql.ErrNoRows`, exported from
  the persistence package for the handler package to check with
  `errors.Is`.
- New `RumourExists` on `internal/store` (a cheap `SELECT EXISTS`) — the
  chosen way to 404 `GET .../comments` on an unknown rumour without
  paying for a full `GetRumourByID` fetch.
- `POST /api/v1/rumours/{id}/comments` (auth required) and
  `GET /api/v1/rumours/{id}/comments` (public) — the first authenticated
  route group in `internal/api/router.go`, wrapping `auth.RequireAuth`
  + a new `httprate.LimitBy(10, time.Minute, auth.KeyByUserID)`, a much
  stricter, per-user-keyed limiter stacked on top of the existing
  shared 60/min-per-IP one on `/api/v1`.
- Verified live against a real running `cmd/api`: full create → list
  round-trip, 401 with no token, 404 on both endpoints for an unknown
  rumour, and the rate limiter actually kicking in at request 11 in a
  minute (`201` ×10, then `429`).

## History and known gaps

Milestone history, the deliberate trade-offs behind the architecture, and the
open gaps in this repo live in the headquarters ledger rather than in this file.
They stay queryable and cost nothing to read until you ask for them.

```sh
cd ~/personal/projects/headquarters
./bin/hq ls --project transfer-scout-api                    # live trade-offs and open gaps
./bin/hq ls --project transfer-scout-api --status superseded # milestone history
./bin/hq show <id>                                          # full text of any entry
```

`superseded` means "no longer current", never "deleted" — every milestone
write-up above Milestone 3.2 is preserved there in full, including the
follow-ups and risk areas that were recorded with it.

## Working practices for this project

- One feature branch per task; conventional commit messages; one PR per
  task referencing the issue it closes.
- Never commit secrets. `.env` is gitignored; only `.env.example` is
  committed.
- Before every commit: `go build ./...` and `go vet ./...` must pass. Add
  tests where it makes sense (extraction parsing, dedup logic, status
  transition logic).
- Two testing styles are in use, depending on what's being verified:
  business logic gets a fake-backed unit test (see `internal/ingest`,
  `internal/cluster` — a small interface + in-memory fake, no network/DB);
  actual SQL correctness gets a real-Postgres integration test guarded by
  `t.Skip` when `DATABASE_URL` is unset (see
  `internal/store/integration_test.go`), so `go test ./...` still passes
  without a database running. To actually run them, the database needs
  migrations applied **and** `seed/seed.sql` loaded (`make migrate-up`
  then `make seed`) — several tests rely on the seeded sources and clubs,
  which migrations don't create. CI does the same (see
  `.github/workflows/ci.yml`).
- `internal/store` is the only package that knows about pgx errors. A
  lookup of a missing row returns an entity sentinel
  (`store.ErrRumourNotFound`, `store.ErrClubNotFound`) — translated from
  `pgx.ErrNoRows` or a `23503` FK violation inside the store — and
  handlers map it to `404` with `errors.Is`. Never import pgx in
  `internal/api` to check for not-found.
- PRs are left open for the project owner to review — do not self-merge.

