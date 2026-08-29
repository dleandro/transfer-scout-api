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
  ticker), `cmd/extract` (LLM extraction worker — one-shot batch, not a
  ticker loop; calls the Anthropic Messages API, see Milestone 1.3).
  `cmd/migrate` is a fourth, dev-only binary wrapping golang-migrate for
  `make migrate-up`/`make migrate-down` — not part of the three application
  binaries.
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

## Current status (as of Milestone 1.6 — Milestone 1 complete)

**2026-07-28: the api repo arrived on GitHub with no scaffolding** — just an
auto-generated README and one "Initial commit", despite the original brief
assuming initial code already existed. This was confirmed with the project
owner, who asked for the scaffolding to be built from scratch as Milestone
0 (GitHub issue #1), ahead of the numbered Milestone 1 task list ("Milestone
1 task 1: fix compilation" was subsumed into building the scaffolding
correctly the first time). See the issue-number map at the bottom of this
file — issue numbers do not run sequentially with milestone numbers, because
one issue-creation command was accidentally skipped and re-filed later.

PRs in this project are **stacked**: each milestone's branch is based on the
previous milestone's branch (not `main`), so they can be reviewed and merged
in order. Do not rebase a later branch onto `main` while earlier PRs in the
stack are still open.

Milestone 0 delivered a working, verified-end-to-end skeleton:

- Schema (`migrations/0001_init_schema.{up,down}.sql`): `clubs`, `players`,
  `sources`, `articles`, `rumours`, `rumour_events`, plus the
  `rumour_status` enum. Verified: migrate up, migrate down (drops back to
  just `schema_migrations`), migrate up again.
- Seed data (`seed/seed.sql`): 20 PL clubs, 10 news sources. **Sources are
  seeded without `feed_url`** (nullable column) — real RSS URLs are
  Milestone 1.2's job. No players or rumours are seeded; those are created
  by the extraction/upsert pipeline (Milestones 1.3/1.4).
- `cmd/api`: chi router, `/healthz`, `GET /api/v1/rumours`,
  `GET /api/v1/rumours/{id}`. **Milestone 1.5** enriched both: responses
  now nest `player`/`to_club`/`from_club` objects (id, name, crest_url)
  instead of raw `*_id` UUIDs, and the detail endpoint's `events` array
  nests each event's `source` (id, name) and `article` (id, url, title).
  `store.ListRumours`/`GetRumourByID` now do the JOINs (`RumourFeedItem`/
  `RumourEventItem` in `internal/store/rumours.go`); `internal/api/views.go`
  maps them to the nested JSON shape. Smoke-tested live against Postgres,
  including a real rumour created through the actual `cluster.Upsert` path
  (not a raw SQL insert) against a live-ingested article, to prove the full
  response shape end-to-end — via a throwaway, uncommitted `cmd/seedtest`
  program, deleted after use.
- `cmd/ingest`: full poller — tickers, fetches each source's feed via
  gofeed, stores articles deduped on URL (`ON CONFLICT (url) DO NOTHING`).
  **Milestone 1.2** populated real, verified-reachable RSS feed URLs for 8
  of the 10 seeded sources (`seed/seed.sql`) — talkSPORT and The Athletic
  have no public feed (JS SPA / paywalled) and are left `NULL`. Verified
  end-to-end against live feeds: a real poll stored 331 articles across the
  8 sources, and a second poll immediately after stored zero new rows
  (dedup confirmed on real data, not just the unit test fixture).
- `cmd/extract`: **Milestone 1.3** implemented `extract.AnthropicExtractor`
  (`internal/extract/anthropic.go`) — calls the Anthropic Messages API with
  `extract.SystemPrompt`, strips markdown code fences the model sometimes
  wraps JSON in, parses into `extract.Result`, and validates status/
  confidence before accepting it. `cmd/extract` runs one batch (50 articles)
  per invocation — not a ticker loop like ingest; schedule it externally
  (cron/systemd timer). Falls back to `extract.StubExtractor` (always
  `ErrNotImplemented`) if `EXTRACT_API_KEY` is unset, so it's safe to run
  in dev without a key. New nullable `articles.extraction JSONB` column
  (migration 0002) holds the raw successful result; `articles.processed`
  is set `true` either way, so failed/unusable articles aren't retried
  forever. **Not yet exercised against a real model call** — no
  `ANTHROPIC_API_KEY` was available in the session that built this
  (checked; not present in the environment). Covered instead by:
  1. Unit tests (`internal/extract/anthropic_test.go`) mocking the
     Anthropic response shape via `httptest` — success, code-fence
     stripping, invalid status, out-of-range confidence, non-200 API
     errors, malformed JSON.
  2. A live run of `cmd/extract` in stub mode against the 331 real
     articles from Milestone 1.2 — confirmed it correctly marks a batch of
     50 processed, leaves 281 unprocessed, and stores no `extraction` JSON
     (since the stub always fails), proving the list/mark-processed/
     persist plumbing works against real Postgres. **If you have a real
     `EXTRACT_API_KEY`, running `cmd/extract` for real is the one
     remaining gap** — do that before trusting the Anthropic integration
     in production.
- Rumour upsert/clustering: **Milestone 1.4** implemented `internal/cluster`
  (`Clusterer.Upsert`) — resolves player/to-club/from-club names to IDs via
  new `store.GetOrCreateClub`/`GetOrCreatePlayer` (case-insensitive exact
  match, `ON CONFLICT (lower(name)) DO UPDATE ... RETURNING id`; **not**
  fuzzy/alias matching — "Man United" and "Manchester United" would create
  two separate club rows), then `store.UpsertRumour` on
  (player_id, to_club_id, transfer_window) and `store.InsertRumourEvent`
  (idempotent per (rumour_id, article_id)). Status only ever moves forward
  (`models.RumourStatus.IsForwardTransition`); the fee range widens to span
  every reported figure via Postgres `LEAST`/`GREATEST` (NULL-ignoring).
  Migration 0003 makes `players.name` case-insensitively unique (players
  didn't have that constraint before — needed for reliable get-or-create;
  known limitation: two distinct real players sharing an exact name would
  incorrectly merge, acceptable for MVP). `cmd/extract` now calls
  `Clusterer.Upsert` after every successful extraction, before marking the
  article processed. **Not wrapped in a transaction** — safe under
  `cmd/extract`'s current single-process sequential-batch execution, but
  would race if extraction were ever parallelized (see the comment on
  `UpsertRumour`).
- Source reliability: **Milestone 1.6** implemented it. `store.UpsertRumour`
  now returns `justResolved bool` — true exactly once, on the upsert call
  that first moves a rumour from a non-terminal status into `confirmed`/
  `collapsed` (every later report on an already-terminal rumour gets
  `justResolved=false`, since `IsForwardTransition` already refuses to
  move a terminal status anywhere). `Clusterer.Upsert` inserts the
  resolving article's own event first, then — only when `justResolved` —
  calls new `store.NudgeSourceReliability(rumourID, delta)`, which bumps
  `reliability_score` (clamped to [0, 100]) for every distinct source
  with an event on that rumour: `+2` for confirmed, `-2` for collapsed
  (`cluster.reliabilityNudge`). `RumourFeedItem` gained a `Credibility
  *float64` field — the average `reliability_score` across a rumour's
  contributing sources, computed via a correlated subquery in
  `rumourFeedSelect` — exposed as `credibility` in both API responses
  (`GET /api/v1/rumours` and `/rumours/{id}`), omitted if nil.
  **Simplification, documented in the `reliabilityNudge` comment**: every
  contributing source gets the same nudge regardless of which status they
  personally reported (e.g. a source that only ever reported "rumoured"
  still gets the "confirmed" credit if the deal later goes through) — this
  doesn't distinguish "accurately reported a deal that fell through" from
  "got it wrong" on a collapsed rumour. Real-world nuance considered out
  of scope for MVP.

### Known follow-ups / risk areas

- Club/player name matching in `internal/cluster` is exact (case-
  insensitive) string matching, not fuzzy/alias resolution. A model
  returning "Spurs" vs. "Tottenham Hotspur", or "Man Utd" vs. "Manchester
  United", will create separate club rows instead of clustering correctly.
  Worth a proper alias table if this turns out to matter in practice once
  real extractions are running.
- `internal/api` view-mapping (`newRumourView`/`newRumourEventView`) has
  unit test coverage (`internal/api/views_test.go`, Milestone 1.5); the
  HTTP handlers themselves (`handleListRumours`/`handleGetRumour`) do not
  — still only smoke-tested manually. `store.ListRumours`/`GetRumourByID`
  have real coverage via `internal/store/integration_test.go` (Milestone
  1.4), which also resolved the pgx NUMERIC/enum scanning risks noted in
  earlier versions of this doc.
- `crest_url` is never populated anywhere yet (seed data doesn't set it,
  nothing writes it) — the enriched API responses correctly omit it
  (`omitempty`) but every club's crest is effectively unset today. Not a
  bug, just unimplemented: nothing in Milestone 1's scope populates crest
  URLs.

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
  without a database running.
- PRs are left open for the project owner to review — do not self-merge.

## Milestone 1 task list and issue numbers

Issue numbers are out of order with milestone numbers — #6 was never
created (a `cd` failure silently dropped it from a batch of `gh issue
create` calls) and was re-filed as #7 afterward.

| Milestone | Issue | Status | Task |
|-----------|-------|--------|------|
| 1.2 | #7 | done | Real RSS feed URLs for seeded sources; verify `cmd/ingest` stores articles end-to-end. |
| 1.3 | #2 | done | LLM extraction worker (`cmd/extract`): pull unprocessed articles, call the model with `extract.SystemPrompt`, parse the JSON, mark articles processed. |
| 1.4 | #3 | done | Rumour upsert + clustering: map extracted club/player names to IDs (create if missing), upsert on (player_id, to_club_id, transfer_window), append a `rumour_event`, handle status transitions and fee-range updates. |
| 1.5 | #4 | done | Flesh out the API: full event timeline + player/club enrichment (names, crests) on `GET /api/v1/rumours/{id}` and the feed response. |
| 1.6 | #5 | done | Source-reliability scoring: nudge source reliability when a rumour resolves (confirmed/collapsed); expose a per-rumour credibility indicator. |

All of Milestone 1 is now implemented and stacked as PRs #6 (Milestone 0),
#8, #9, #10, #11, and #12 (Milestones 1.2 through 1.6, in order — #7 is an
issue number, not a PR), each based on the previous, waiting for
review/merge in order. Next: pick a Milestone 2 (nothing defined yet as of
this writing) — e.g. the prediction game, premium subscriptions, or web
frontend work in `transfer-scout-web` (currently just an auto-generated
README, same as this repo was before Milestone 0).
