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
- Three binaries: `cmd/api` (REST API), `cmd/ingest` (RSS poller on a
  ticker), `cmd/extract` (LLM extraction worker — currently a **stub**, see
  Milestone 1.3). `cmd/migrate` is a fourth, dev-only binary wrapping
  golang-migrate for `make migrate-up`/`make migrate-down` — not part of
  the three application binaries.
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
- PL only for the MVP. Current window: `summer-2026` (`TRANSFER_WINDOW` env
  var, defaults to this in `internal/config`).

## Current status (as of Milestone 0)

**2026-07-28: the api repo arrived on GitHub with no scaffolding** — just an
auto-generated README and one "Initial commit", despite the original brief
assuming initial code already existed. This was confirmed with the project
owner, who asked for the scaffolding to be built from scratch as Milestone
0 (GitHub issue #1), ahead of the numbered Milestone 1 task list (issues
#2-#5, corresponding to Milestone 1 tasks 2-6 in the original brief;
"Milestone 1 task 1: fix compilation" was subsumed into building the
scaffolding correctly the first time).

Milestone 0 delivered a working, verified-end-to-end skeleton:

- Schema (`migrations/0001_init_schema.{up,down}.sql`): `clubs`, `players`,
  `sources`, `articles`, `rumours`, `rumour_events`, plus the
  `rumour_status` enum. Verified: migrate up, migrate down (drops back to
  just `schema_migrations`), migrate up again.
- Seed data (`seed/seed.sql`): 20 PL clubs, 10 news sources. **Sources are
  seeded without `feed_url`** (nullable column) — real RSS URLs are
  Milestone 1.2's job. No players or rumours are seeded; those are created
  by the extraction/upsert pipeline (Milestones 1.3/1.4).
- `cmd/api`: chi router, `/healthz`, `GET /api/v1/rumours` (list, no
  enrichment yet), `GET /api/v1/rumours/{id}` (rumour + raw event list, no
  enrichment yet). Smoke-tested live against Postgres.
- `cmd/ingest`: full poller — tickers, fetches each source's feed via
  gofeed, stores articles deduped on URL (`ON CONFLICT (url) DO NOTHING`).
  Not yet exercised against a real feed because no source has a `feed_url`
  yet — that's Milestone 1.2.
- `cmd/extract`: honest stub. Reports the count of unprocessed articles and
  calls `extract.StubExtractor`, which always returns
  `extract.ErrNotImplemented`. No model integration yet — Milestone 1.3.
- Rumour upsert/clustering: **not implemented** — Milestone 1.4. Nothing
  currently turns an article into a rumour.
- Source reliability: schema field exists (`sources.reliability_score`,
  default 50.00) but nothing updates it yet — Milestone 1.6.

### Known follow-ups / risk areas

- `Rumour.FeeMinEUR/FeeMaxEUR/Confidence` and `RumourEvent` equivalents are
  scanned directly from Postgres `NUMERIC` into `*float64` via pgx v5's
  default conversion. This compiles and is a common pattern, but hasn't
  been exercised against real data yet (no rumours exist until Milestone
  1.4). If pgx scan errors show up once rumours are populated, switch to
  `pgtype.Numeric` and convert explicitly.
- `RumourStatus` (a Go string-kind type) is scanned directly from the
  Postgres `rumour_status` enum without explicit `pgtype` registration,
  relying on pgx v5's text-format fallback for unregistered types. Same
  caveat — not yet exercised with real rows.
- `internal/api` has no test coverage yet (handlers are thin; store methods
  are the part worth testing once there's real data flowing through them).

## Working practices for this project

- One feature branch per task; conventional commit messages; one PR per
  task referencing the issue it closes.
- Never commit secrets. `.env` is gitignored; only `.env.example` is
  committed.
- Before every commit: `go build ./...` and `go vet ./...` must pass. Add
  tests where it makes sense (extraction parsing, dedup logic, status
  transition logic).
- PRs are left open for the project owner to review — do not self-merge.

## Milestone 1 task list (GitHub issues #2-#5 in this repo)

2. Populate real RSS feed URLs for the seeded sources; verify `cmd/ingest`
   stores articles end-to-end.
3. Implement the LLM extraction worker (`cmd/extract`): pull unprocessed
   articles, call the model with `extract.SystemPrompt`, parse the JSON,
   mark articles processed.
4. Rumour upsert + clustering: map extracted club/player names to IDs
   (create if missing), upsert on (player_id, to_club_id, transfer_window),
   append a `rumour_event`, handle status transitions and fee-range
   updates.
5. Flesh out the API: full event timeline + player/club enrichment (names,
   crests) on `GET /api/v1/rumours/{id}` and the feed response.
6. Source-reliability scoring: nudge source reliability when a rumour
   resolves (confirmed/collapsed); expose a per-rumour credibility
   indicator.
