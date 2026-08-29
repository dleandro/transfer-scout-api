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

## Current status (as of Milestone 3.1)

**2026-08-06: Milestone 3.1 (users, Google OAuth verification, JWT
issuance)** — built on top of the `production/*` stack (this repo's
Cloud-Run/Docker/CI readiness work), not on the Milestone 1.4–1.6 or 2.1
branches, which also touch this repo. First identity/auth concept
anywhere in this codebase.

- New `users` table (migration `0003_add_users`), keyed on `google_sub`
  (Google's durable subject identifier) — `email` is intentionally NOT
  unique, since it isn't guaranteed permanently stable.
- New `internal/auth` package: `GoogleVerifier` (wraps
  `github.com/coreos/go-oidc/v3`, constructed once at `cmd/api` startup —
  it fetches Google's OIDC discovery document and caches signing keys,
  so it's deliberately not a stateless per-call function), `IssueToken`/
  `ParseToken` (HS256 JWT via `github.com/golang-jwt/jwt/v5`, 30-day TTL,
  a reserved-but-unused `jti` claim), and `RequireAuth` middleware +
  `KeyByUserID` (an `httprate.KeyFunc` for a future per-user rate limiter
  on mutating endpoints — see Milestone 3.2/3.3).
- New `POST /api/v1/auth/google` (public, under the existing shared
  60/min IP limiter): verifies a Google ID token, upserts the user,
  returns a Transfer-Scout JWT. Chosen library
  (`github.com/coreos/go-oidc/v3` + `golang.org/x/oauth2`) over the
  official `google.golang.org/api/idtoken` for a much lighter dependency
  tree (`go mod tidy` only added `go-jose` and `x/oauth2` as new
  transitive deps).
- `AUTH_JWT_SECRET`/`GOOGLE_CLIENT_ID` are `cmd/api`-only — deliberately
  NOT validated inside the shared `config.Load()` (which `cmd/ingest`/
  `cmd/extract` also call and would otherwise fail to start over env
  vars they never use); `cmd/api/main.go` checks them itself right after
  `Load()`, same fail-fast spirit as `DatabaseURL` without breaking the
  other two binaries.
- **Not tested end-to-end against a real Google account** — no OAuth
  Client ID/Secret was available in the session that built this (same
  situation `cmd/extract`'s Anthropic integration was in when first
  built). Verified instead: `IssueToken`/`ParseToken` round-trip and
  expiry, `RequireAuth`'s missing/malformed/expired/valid cases,
  `UpsertUser`'s insert-then-update behavior (real-Postgres integration
  test), and `POST /api/v1/auth/google`'s error paths (missing/invalid
  `id_token` → 400/401) live against a real `cmd/api` process — including
  confirming `NewGoogleVerifier` successfully reaches Google's real OIDC
  discovery endpoint at startup with a dummy client ID. The one thing
  *not* exercised is a real, valid Google ID token reaching
  `handleGoogleAuth` successfully. If you have real Google OAuth
  credentials, running that end-to-end is the one remaining gap.
- **Local dev DB note**: while testing this migration, the long-running
  local dev Postgres (used throughout this repo's history) was found to
  already have `schema_migrations.version = 3` (dirty=false) despite no
  migration 3 existing in the repo before now — meaning `migrate up`
  silently no-op'd against it instead of creating `users`. Cause unknown
  (not introduced by this change); verified this migration is correct by
  running it against a clean throwaway Postgres instead. If you hit a
  missing `users` table against the long-running local dev DB, this is
  why — worth a proper investigation/reset of that DB at some point.

### Known follow-ups / risk areas (Milestone 3.1)

- No token refresh flow: the Google `id_token` NextAuth receives is only
  available at initial sign-in, so the 30-day Transfer-Scout JWT can't be
  silently refreshed — when it expires, the user just signs in again.
  Explicit trade-off, not a bug.
- No server-side sign-out/revocation: the JWT is stateless, so signing
  out only clears the frontend's session cookie. A `jti` claim is
  reserved (generated, unused) so a future revocation table wouldn't
  need a token-shape change.
- `handleGoogleAuth` has no fake-backed handler test (unlike
  `handleListRumours`/`handleGetRumour`) — `GoogleVerifier` wraps real
  network calls to Google that aren't easily faked without standing up a
  mock OIDC server; covered by the live-process verification above
  instead.

## Current status (as of Milestone 2.1)

**2026-08-05: Milestone 2.1 (`GET /api/v1/rumours` pagination)** — originally
built on top of Milestone 1.3's `main`, in parallel with the (then still
open) Milestone 1.4–1.6 branches, which also touched
`internal/api/handlers.go` / `internal/store/rumours.go` for feed
enrichment and credibility. Milestone 1.4–1.6 merged first; this milestone
required the flagged manual reconciliation on merge, combining the
limit+1-and-trim pagination trick with the enriched `RumourFeedItem`
query/view path (`store.ListRumours` now returns
`([]RumourFeedItem, hasMore bool, err error)`).

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
  instead of a bare JSON array. `rumours` is `[]`, not `null`, when the
  page is empty — the enriched response builds `views` via `make()` over
  the (possibly nil) item slice, so the old nil-slice-to-`null` quirk no
  longer applies at this nesting level; callers should not rely on it.
  - `has_more` is computed by `store.ListRumours` requesting `limit+1`
    rows and trimming the extra one if present, rather than a separate
    `COUNT(*)` query.
- `GET /api/v1/rumours/{id}` is unchanged by this milestone.
- Tests: `internal/api/handlers_test.go` unit-tests `parseIntParam`'s
  parsing/clamping directly (no store dependency, so no fake needed).
  `internal/store/integration_test.go` proves the `limit+1`-and-trim trick
  against a real Postgres — guarded by `t.Skip` when `DATABASE_URL` is
  unset, per this repo's usual integration-test convention.

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
