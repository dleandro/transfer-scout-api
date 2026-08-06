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
- Three binaries: `cmd/api` (REST API), `cmd/ingest` (RSS poller —
  one-shot batch per invocation as of the production roadmap's Task 5.1;
  originally an in-process ticker loop, changed so Cloud Scheduler can own
  cadence by triggering a Cloud Run Job, see PRODUCTION_ROADMAP.md),
  `cmd/extract` (LLM extraction worker — one-shot batch, not a ticker
  loop; calls the Anthropic Messages API, see Milestone 1.3). `cmd/migrate`
  is a fourth, dev-only binary wrapping golang-migrate for
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

## Current status (as of Milestone 2.1)

**2026-08-05: Milestone 2.1 (`GET /api/v1/rumours` pagination)** — built on
top of Milestone 1.3's `main`, since the base list endpoint was already
merged there; not stacked on the (still-open, as of this writing)
Milestone 1.4–1.6 branches, which also touch `internal/api/handlers.go` /
`internal/store/rumours.go` for feed enrichment and credibility. Whichever
of these merges second will need a small manual reconciliation — flagged,
not silently avoided.

- `GET /api/v1/rumours` now takes `limit` and `offset` query params
  (`strconv.Atoi`) instead of a hardcoded `limit=50, offset=0`.
  - `limit`: default 50. Clamped into `[1, 100]` if a valid integer
    outside that range is given (e.g. `limit=500` silently becomes 100)
    — a deliberate choice to avoid a client requesting an unbounded page.
  - `offset`: default 0. Negative values are clamped to 0.
  - A value that isn't a valid integer at all (e.g. `limit=abc`) is
    treated differently from an out-of-range one: it's a 400 Bad
    Request, not a silent fallback, since it usually indicates a client
    bug worth surfacing rather than a legitimate large/negative request.
  - See `parseIntParam` in `internal/api/handlers.go`.
- **Response shape changed** (breaking change to the previous contract,
  coordinated with transfer-scout-web in the same milestone): `GET
  /api/v1/rumours` now returns `{"rumours": [...], "has_more": bool}`
  instead of a bare JSON array. `rumours` can still be `null` (not `[]`)
  when the page is empty — same Go nil-slice-marshals-to-null quirk as
  before, now nested one level down; callers need to null-coalesce
  `data.rumours`, not just the top-level response.
  - `has_more` is computed by `store.ListRumours` requesting `limit+1`
    rows and trimming the extra one if present, rather than a separate
    `COUNT(*)` query.
- `GET /api/v1/rumours/{id}` is unchanged by this milestone.
- Tests: `internal/api/handlers_test.go` unit-tests `parseIntParam`'s
  parsing/clamping directly (no store dependency, so no fake needed).
  `internal/store/integration_test.go` (new file on this branch) proves
  the `limit+1`-and-trim trick against a real Postgres — guarded by
  `t.Skip` when `DATABASE_URL` is unset, per this repo's usual
  integration-test convention.

## Current status (as of Milestone 1.3)

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
- `cmd/api`: chi router, `/healthz`, `GET /api/v1/rumours` (list, no
  enrichment yet), `GET /api/v1/rumours/{id}` (rumour + raw event list, no
  enrichment yet). Smoke-tested live against Postgres.
- `cmd/ingest`: fetches each source's feed via gofeed, stores articles
  deduped on URL (`ON CONFLICT (url) DO NOTHING`). Originally an in-process
  ticker loop; converted to one-shot-per-invocation by the production
  roadmap's Task 5.1 so Cloud Scheduler can own cadence — see
  PRODUCTION_ROADMAP.md.
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
- Rumour upsert/clustering: **not implemented** — Milestone 1.4. Nothing
  currently turns an article's extraction into a rumour. Because no live
  model call has happened yet, there's no real `articles.extraction` data
  to upsert from either — Milestone 1.4 will need to seed some synthetic
  extraction JSON (or a fake `extract.Extractor`) to test against.
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

## Milestone 1 task list and issue numbers

Issue numbers are out of order with milestone numbers — #6 was never
created (a `cd` failure silently dropped it from a batch of `gh issue
create` calls) and was re-filed as #7 afterward.

| Milestone | Issue | Status | Task |
|-----------|-------|--------|------|
| 1.2 | #7 | done | Real RSS feed URLs for seeded sources; verify `cmd/ingest` stores articles end-to-end. |
| 1.3 | #2 | done | LLM extraction worker (`cmd/extract`): pull unprocessed articles, call the model with `extract.SystemPrompt`, parse the JSON, mark articles processed. |
| 1.4 | #3 | pending | Rumour upsert + clustering: map extracted club/player names to IDs (create if missing), upsert on (player_id, to_club_id, transfer_window), append a `rumour_event`, handle status transitions and fee-range updates. |
| 1.5 | #4 | pending | Flesh out the API: full event timeline + player/club enrichment (names, crests) on `GET /api/v1/rumours/{id}` and the feed response. |
| 1.6 | #5 | pending | Source-reliability scoring: nudge source reliability when a rumour resolves (confirmed/collapsed); expose a per-rumour credibility indicator. |
