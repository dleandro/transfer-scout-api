# Transfer Scout — Production Roadmap

Context for whoever (human or Claude session) picks this up: this repo pair
has working product logic but zero deployment story. This doc is a
dependency-ordered task list to take it to production. Each task is meant
to be actionable on its own — read its Context before starting, and check
the "Ground state" section below first since both repos use stacked PRs
whose tips move over time.

Every task below also exists as a GitHub issue (label `production`) in the
repo it belongs to, so you can work from issues alone without this file
open — this file is the canonical full-detail version they link back to.

**Locked decisions (do not re-litigate):**
- API (`cmd/api`) → Google Cloud Run service (Docker). `cmd/ingest` /
  `cmd/extract` → Cloud Run Jobs, triggered by Cloud Scheduler.
- Database → Neon (free serverless Postgres, real wire-protocol compatible
  with pgx — no driver changes needed, just connection strings).
- Frontend → Vercel free Hobby tier (deliberately not Dockerized — best
  fit for Next.js specifically; every other service uses Docker).
- Auth → none. API stays fully public/read-only for this launch. Do not
  add auth/API-key middleware as part of this roadmap.
- CI → GitHub Actions for both repos (lint/vet/build/test on PRs).
- Containers → Docker everywhere it applies (not Apple's macOS `container`
  CLI).

## Ground state (check before starting any task — branches move)

- **transfer-scout-api**: `main` only has Milestones 0–1.3 merged. Open,
  stacked PRs: #10 (`milestone-1.4/rumour-upsert`), #11
  (`milestone-1.5/api-enrichment`, based on #10), #12
  (`milestone-1.6/source-reliability`, based on #11), #14
  (`milestone-2.1/rumour-pagination`, based on 1.3 directly — a separate
  stack). Repo HEAD as of this writing: `milestone-2.1/rumour-pagination`.
  Milestone issues: closed #1, #2, #7; open #3, #4, #5, #13 (implementation
  already exists on the unmerged PRs above — open because unreviewed, not
  because undone).
- **transfer-scout-web**: `main` is just the auto-generated scaffold —
  **even Milestone 0 (PR #2) is unmerged.** Stack: `milestone-0/scaffolding`
  → `milestone-2.2/rumour-detail-page` (PR #5) → `milestone-2.3/feed-pagination`
  (PR #6). Repo HEAD as of this writing: `milestone-2.3/feed-pagination`.
  **The rumour detail page and feed pagination UI already exist** on this
  branch (`src/app/rumours/[id]/page.tsx`, pagination params in
  `src/lib/api.ts`'s `getRumours`) — don't re-implement them. Milestone
  issues: no closed issues; open #1, #3, #4 (same caveat — implementing PRs
  #2, #5, #6 are open, work is done).
- Both repos: one feature branch per task, PRs stay open for the project
  owner to review (never self-merge), commits use conventional messages.
  **Start every task below on a new branch stacked on that repo's current
  PR-chain tip**, not off `main` — run `git branch -a` and
  `gh pr list --state open` first to confirm the tip hasn't moved.
- Because `main` lags behind in both repos, **CI must trigger on
  `pull_request` regardless of base branch**, not just against `main`.

---

## Phase 1 — Containerization & Local Docker Parity (transfer-scout-api)

### Task 1.1 — Multi-stage Dockerfile for all four binaries
- **Repo**: transfer-scout-api
- **Why**: no `Dockerfile` exists anywhere. Cloud Run (service + jobs) needs
  a container image. The module builds four binaries (`cmd/api`,
  `cmd/ingest`, `cmd/extract`, `cmd/migrate`) from one `go.mod` — decide the
  image strategy once, here, so later tasks don't re-derive it.
- **Scope**:
  - New `transfer-scout-api/Dockerfile`. Builder stage `golang:1.25` (matches
    `go.mod`'s `go 1.25.0`), `CGO_ENABLED=0 go build` each of the four
    `cmd/*` packages to `/out/{api,ingest,extract,migrate}`. Final stage:
    `gcr.io/distroless/static-debian12` (pgx v5 + gofeed are pure Go, no
    CGO/glibc needed, so distroless-static works and is smaller/safer than
    alpine).
  - **Decision, document it in a Dockerfile header comment**: one image with
    all four binaries at `/app/{api,ingest,extract,migrate}`, `WORKDIR /app`,
    default `CMD ["/app/api"]`; Cloud Run resources select the binary via a
    `command` override (Task 4.4 uses the default, 5.4 overrides it).
    Rejected alternative — separate image per binary — not worth the
    registry/CI overhead for four small static binaries sharing one module
    with no independent release cadence.
  - `COPY migrations/ /app/migrations` — required because
    `cmd/migrate/main.go` resolves `file://migrations` relative to CWD at
    runtime, not compile time; must land at the same relative path with
    `WORKDIR /app` set.
  - New `transfer-scout-api/.dockerignore` excluding `.env` and `.git`
    (`.env.example`, `migrations/`, `seed/` should stay).
- **Acceptance criteria**: `docker build -t transfer-scout-api .` succeeds.
  `docker run --rm -e DATABASE_URL=bogus transfer-scout-api` fails with a DB
  connect error (proves the default `/app/api` entrypoint runs, not "binary
  not found"). Same check via `--entrypoint /app/ingest`,
  `--entrypoint /app/extract`, `--entrypoint /app/migrate -- up`.
- **Dependencies**: none — first task.

### Task 1.2 — Docker Compose parity for the full local stack
- **Repo**: transfer-scout-api
- **Why**: existing `docker-compose.yml` only has a `db` service; there's no
  way to run the built image locally before deploying it.
- **Scope**: add an `api` service (`build: .`, `depends_on: db` using the
  existing healthcheck condition, `env_file: .env`, port `8080:8080`). Add
  `ingest`/`extract`/`migrate` services under a compose `profiles: ["jobs"]`
  tag so `docker compose up` doesn't start them but
  `docker compose run --rm <name>` does — this intentionally mirrors the
  Cloud Run "Service vs Job" split before it exists in GCP. Add `Makefile`
  targets: `docker-build`, `docker-up`, `docker-migrate`, `docker-ingest`,
  `docker-extract`.
- **Acceptance criteria**: `docker compose up db api` serves
  `/healthz`/`/api/v1/rumours` against the composed Postgres;
  `docker compose run --rm migrate` applies migrations in-container.
- **Dependencies**: Task 1.1.

---

## Phase 2 — Continuous Integration

### Task 2.1 — GitHub Actions CI for transfer-scout-api
- **Repo**: transfer-scout-api
- **Why**: no `.github/workflows` exists; lint/vet/build/test should be
  enforced on PRs.
- **Scope**: `transfer-scout-api/.github/workflows/ci.yml`. Trigger on
  `pull_request` (any base) + `push` to `main`. Steps: checkout →
  `actions/setup-go` (`go-version-file: go.mod`) → `go build ./...` →
  `go vet ./...` → `go test ./...`. Add a `postgres:16` service container
  (matches `docker-compose.yml`'s image) so the `t.Skip`-guarded tests in
  `internal/store/integration_test.go` actually run: export `DATABASE_URL`
  pointing at it, `go run ./cmd/migrate up` before `go test`. Add a
  `docker build .` step as a drift-detector against Task 1.1's Dockerfile.
- **Acceptance criteria**: PRs trigger the workflow; a broken build/vet/test
  fails the check; integration tests show as executed, not skipped, in CI.
- **Dependencies**: Task 1.1.

### Task 2.2 — GitHub Actions CI for transfer-scout-web
- **Repo**: transfer-scout-web
- **Why**: no CI exists; `pnpm build/lint/test`/`tsc --noEmit` reportedly
  pass locally but nothing enforces it.
- **Scope**: `transfer-scout-web/.github/workflows/ci.yml`. Trigger on
  `pull_request` + `push` to `main`. Steps: checkout → `pnpm/action-setup` →
  `actions/setup-node` (pnpm cache) → `pnpm install --frozen-lockfile` →
  `pnpm exec tsc --noEmit` → `pnpm lint` → `pnpm test` → `pnpm build`. Set a
  dummy `API_BASE_URL` for the workflow so `next build` never depends on
  runtime env state (both routes are request-time-only Server Components,
  no build-time fetch/`generateStaticParams`, so this should hold).
- **Acceptance criteria**: PR triggers workflow; all steps pass.
- **Dependencies**: none; land alongside Task 6.1 (shares the env-var
  concern).

---

## Phase 3 — Database & Connection Pooling for Neon + Cloud Run

### Task 3.1 — Provision Neon and apply migrations
- **Repo**: cross-cutting infra, mainly affects transfer-scout-api
- **Why**: Postgres hosting decision is Neon; nothing is provisioned yet.
- **Scope**: create a Neon project. Neon gives both a **direct** (unpooled)
  and a **pooled** (PgBouncer transaction-mode) connection string — record
  both. Use pooled as the runtime `DATABASE_URL` for `cmd/api`/`cmd/ingest`/
  `cmd/extract`; use direct as a separate `DATABASE_MIGRATE_URL` for
  `cmd/migrate` only (DDL + advisory locks behave better unpooled). Run
  `go run ./cmd/migrate up` against the direct URL to apply
  `migrations/0001_*`/`0002_*`; optionally seed via
  `psql "$DATABASE_MIGRATE_URL" -f seed/seed.sql`. Add a "Deployment"
  section to `transfer-scout-api/CLAUDE.md` documenting both connection
  roles (not real secret values). Update `.env.example` with explanatory
  comments (no real values committed).
- **Acceptance criteria**: `schema_migrations` shows both versions applied;
  both connection strings recorded in a secrets store (not git) for Task
  4.4/5.4.
- **Dependencies**: none; blocks Tasks 3.2, 4.x, 5.x.
- **Flag**: Neon's pooled endpoint is PgBouncer in *transaction mode*,
  which can conflict with pgx's prepared-statement caching under
  concurrent pooled connections ("prepared statement already exists").
  Validate this explicitly in Task 3.2 — don't assume the connection
  string swap "just works." Neon's free tier includes point-in-time
  restore within its retention window, which covers the "no backup
  tooling" gap adequately for MVP — no separate backup task needed.

### Task 3.2 — pgxpool tuning for Neon + Cloud Run cold starts
- **Repo**: transfer-scout-api
- **Why**: `internal/db/db.go`'s `New` calls `pgxpool.New` with zero tuning
  (confirmed — no `MaxConns`/`MinConns`/lifetime settings). Cloud Run's
  scale-to-zero means every cold start opens fresh connections; unbounded
  per-instance pools against Neon's free-tier connection cap risk "too many
  connections," compounded by the prepared-statement risk above.
- **Scope**: change `internal/db/db.go`'s `New` to build config via
  `pgxpool.ParseConfig(databaseURL)` before `pgxpool.NewWithConfig`. Set a
  small explicit `MaxConns` (4–5 per instance), `MinConns: 0`, and a
  `MaxConnLifetime`/`MaxConnIdleTime` so idle connections don't outlive
  Neon's autosuspend. If Task 3.1 surfaces prepared-statement errors against
  the pooled endpoint, set
  `poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol`
  here too.
- **Acceptance criteria**: change compiles; `internal/store/integration_test.go`
  (calls `db.New`) still passes; a handful of concurrent `curl`s against a
  deployed `/api/v1/rumours` shows no connection/prepared-statement errors.
- **Dependencies**: Task 3.1 (need the real pooled string to validate against).

---

## Phase 4 — API Deploy to Cloud Run

### Task 4.1 — PORT/env reconciliation
- **Repo**: transfer-scout-api
- **Why**: Cloud Run injects `PORT` and requires the container to listen on
  it. `internal/config/config.go` currently only reads `API_PORT` (default
  `"8080"`, confirmed at `config.go:23`) — it "happens to" match Cloud Run's
  own default port, which is a coincidence, not a contract, and a classic
  Cloud Run "container failed to start" trap.
- **Scope**: change `Config.APIPort` to prefer `PORT`, fall back to
  `API_PORT`, then `"8080"` (e.g.
  `getEnv("PORT", getEnv("API_PORT", "8080"))`). Update `.env.example`/docs
  noting Cloud Run's `PORT` takes precedence.
- **Acceptance criteria**: `PORT=9090 go run ./cmd/api` listens on 9090;
  local `.env`-based `API_PORT` still works as fallback.
- **Dependencies**: none.

### Task 4.2 — `/healthz` DB-connectivity check
- **Repo**: transfer-scout-api
- **Why**: confirmed — `handleHealth` (`internal/api/handlers.go:21-23`) is
  pure liveness (`{"status":"ok"}`), no DB check. DB is only pinged once at
  process startup (`cmd/api/main.go`). Given Neon's autosuspend/resume
  behavior, "process is up" and "DB is reachable" are genuinely different
  facts worth surfacing to Cloud Run's health probe.
- **Scope**: add `Ping(ctx) error` to `internal/store/store.go` (delegates
  to the wrapped `*pgxpool.Pool.Ping`). Change `handleHealth` to call it
  with a ~2s timeout: 200 `{"status":"ok"}` on success, 503
  `{"status":"degraded","db":"unreachable"}` on failure. Add fake-backed
  tests for both branches to `internal/api` (this repo's established
  testing pattern — see `internal/ingest`).
- **Acceptance criteria**: DB up → 200; DB down → 503; new tests pass.
- **Dependencies**: none.

### Task 4.3 — Migration execution strategy: manual Cloud Run Job
- **Repo**: transfer-scout-api / GCP
- **Scope (decision to document, not just implement)**: create a Cloud Run
  Job `transfer-scout-migrate` from the Task 1.1 image, `command` override
  `["/app/migrate", "up"]`, using the **direct** Neon URL from Task 3.1.
  Migrations run as an **explicit, manually-triggered step before any
  deploy that changes schema** (`gcloud run jobs execute
  transfer-scout-migrate`) — not automatically on `cmd/api` boot. Document
  the rejected alternative (migrate-on-boot) in
  `transfer-scout-api/CLAUDE.md`'s Deployment section: Cloud Run can start
  multiple instances concurrently on deploy, and while golang-migrate's
  advisory lock likely makes concurrent `Up()` safe, an explicit auditable
  step is preferred over relying on that implicitly.
- **Acceptance criteria**: `gcloud run jobs execute transfer-scout-migrate`
  applies pending migrations; running it twice is a no-op (already handled
  by `cmd/migrate`'s `migrate.ErrNoChange` check).
- **Dependencies**: Tasks 1.1, 3.1.

### Task 4.4 — Deploy `cmd/api` as a Cloud Run service
- **Repo**: transfer-scout-api / GCP
- **Scope**: push the Task 1.1 image to Artifact Registry; create Cloud Run
  **service** `transfer-scout-api` (no `command` override — image's `CMD`
  is already `/app/api`). Env vars: `DATABASE_URL` (Neon pooled string, as a
  **Secret Manager** reference — the one place a secrets manager is
  actually warranted here), `TRANSFER_WINDOW`. Do not set `EXTRACT_*`/
  `INGEST_*` on this service. `--allow-unauthenticated` (public, per the
  no-auth decision). `min-instances=0`, small `max-instances` (2–3,
  bounding Neon connections given Task 3.2's per-instance caps). Point the
  health check at `/healthz` (Task 4.2).
- **Acceptance criteria**: `curl <url>/healthz` → 200; `curl
  <url>/api/v1/rumours` returns a well-formed response (may be an empty
  page — see the flag under Task 5.5, rumour clustering isn't merged yet).
- **Dependencies**: Tasks 3.1, 3.2, 4.1, 4.2, 4.3.

---

## Phase 5 — Scheduled Jobs (ingest & extract via Cloud Run Jobs + Cloud Scheduler)

### Task 5.1 — Convert `cmd/ingest` from ticker loop to single-shot
- **Repo**: transfer-scout-api
- **Why**: confirmed — `cmd/ingest/main.go:38` calls
  `poller.Run(ctx, cfg.IngestPollInterval)`, an in-process ticker loop
  (`internal/ingest/ingest.go`'s `Run`). Under Cloud Run Jobs, Cloud
  Scheduler owns cadence by triggering fresh executions — the binary must
  poll once and exit.
- **Scope**:
  - `cmd/ingest/main.go`: replace the `poller.Run(...)` call with
    `poller.PollOnce(ctx)`, then exit — mirrors `cmd/extract/main.go`'s
    existing one-shot shape. Keep the existing `signal.NotifyContext` setup
    (already present, confirmed at `cmd/ingest/main.go:25`) so SIGTERM still
    cancels in-flight fetches cleanly.
  - `internal/ingest/ingest.go`: delete `Run`/its ticker loop once nothing
    calls it (check `internal/ingest/ingest_test.go` doesn't test `Run`
    directly first; adapt if it does). `PollOnce` is the real, already-
    tested unit of work — unchanged.
  - `internal/config/config.go`: remove `IngestPollInterval` from `Config`
    and its parsing block (`config.go:33-38`); remove `INGEST_POLL_INTERVAL`
    from `.env.example`.
  - Update stale doc comments describing ingest as ticker-based
    (`internal/ingest/ingest.go:1-3`, `cmd/ingest/main.go:1-2`) and the
    "locked architecture decisions" line in `transfer-scout-api/CLAUDE.md`
    — call this out explicitly in the PR description as a deliberate
    architecture change, not silent drift.
- **Acceptance criteria**: `go run ./cmd/ingest` does exactly one poll pass
  and exits 0; `go build`/`go vet` clean; ingest tests pass.
- **Dependencies**: Task 1.1 (rebuild the image after this change).

### Task 5.2 — Graceful shutdown for `cmd/extract`
- **Repo**: transfer-scout-api
- **Why**: confirmed — `cmd/extract/main.go:30` uses
  `context.Background()`, unlike `cmd/ingest`/`cmd/api` which already use
  `signal.NotifyContext`. Under Cloud Run Jobs, hitting the task timeout or
  being cancelled sends SIGTERM; without honoring it there's no clean
  mid-batch cutoff.
- **Scope**: replace `context.Background()` with the same
  `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`
  + `defer stop()` pattern used elsewhere. In the per-article loop
  (`main.go:58-70`), check `ctx.Err()` at the top of each iteration and
  break cleanly, logging completed-vs-skipped counts so an interrupted run
  is distinguishable from a clean one (non-zero exit on interruption).
- **Acceptance criteria**: SIGTERM mid-batch stops new pickups, logs a clear
  "extract: interrupted" message with counts, exits non-zero.
- **Dependencies**: none; grouped here as prep for scheduling.

### Task 5.3 — Real Anthropic API smoke test for `cmd/extract`
- **Repo**: transfer-scout-api (verification task, minimal/no code change
  expected)
- **Why**: confirmed via `transfer-scout-api/CLAUDE.md` — `cmd/extract`'s
  real Anthropic call path has never been exercised (only mocked-`httptest`
  and stub-fallback runs so far). Must be closed before letting it run
  unattended on a schedule.
- **Scope**: obtain a real `EXTRACT_API_KEY`, run `cmd/extract` against real
  unprocessed articles (331 were ingested in Milestone 1.2). Verify: real
  response JSON parses, the code-fence-stripping logic in
  `extract.AnthropicExtractor` behaves as expected either way, status/
  confidence validation passes on real outputs, `articles.extraction` gets
  populated. Any shape mismatch found becomes a follow-up bug task with a
  corresponding test update — don't silently patch around it.
- **Acceptance criteria**: at least one real batch run with `extracted > 0`
  and no unexpected failures; note added to `transfer-scout-api/CLAUDE.md`'s
  "Known follow-ups" closing (or documenting) this gap.
- **Dependencies**: Task 3.1 (real Postgres with real unprocessed articles).
  Must finish before Task 5.5 goes live — don't schedule an unverified
  extractor.

### Task 5.4 — Create Cloud Run Jobs for ingest and extract
- **Repo**: transfer-scout-api / GCP
- **Scope**: using the rebuilt (post 5.1/5.2) Task 1.1 image:
  - `transfer-scout-ingest`: `command` override `/app/ingest`; env
    `DATABASE_URL` (pooled — short insert bursts are fine pooled); timeout
    sized for a full RSS sweep (5–10 min); retries 0–1 are safe since
    `InsertArticle`'s `ON CONFLICT (url) DO NOTHING` makes reruns idempotent.
  - `transfer-scout-extract`: `command` override `/app/extract`; env
    `DATABASE_URL` (pooled), `EXTRACT_API_KEY` (**Secret Manager** — the one
    genuinely sensitive credential in this system), `EXTRACT_MODEL`; timeout
    sized for a 50-article batch's worst-case LLM latency. Confirm before
    setting retries > 0: `MarkExtracted` sets `processed=true` regardless of
    per-article extraction success (`cmd/extract/main.go:67-69`), so a
    job-level retry after a mid-run crash re-lists a fresh
    `ListUnprocessed` batch and is likely benign — verify this holds rather
    than assuming it.
- **Acceptance criteria**: `gcloud run jobs execute` for both jobs completes
  and produces the expected DB changes (spot-check via `psql`/API).
- **Dependencies**: Tasks 5.1, 5.2, 5.3, 3.1, and Secret Manager wiring
  analogous to Task 4.4's.

### Task 5.5 — Cloud Scheduler cron triggers
- **Repo**: cross-cutting / GCP
- **Why**: closes "nothing schedules `cmd/extract`" and replaces the old
  in-process `INGEST_POLL_INTERVAL` (removed in Task 5.1) with an
  externally-owned cadence. Free tier allows 3 scheduler jobs; this plan
  uses 2.
- **Scope**: two Cloud Scheduler jobs, each HTTP-triggering its Cloud Run
  Job's run endpoint:
  - `ingest-schedule`: every 5 minutes (`*/5 * * * *`, matching the old
    default `INGEST_POLL_INTERVAL=5m` so behavior doesn't regress).
  - `extract-schedule`: every 10–15 minutes (no prior precedent — tune
    against Task 5.3's observed Anthropic latency/cost).
  Document both schedules in `transfer-scout-api/CLAUDE.md`'s Deployment
  section.
- **Acceptance criteria**: both scheduler jobs show successful executions
  over at least one full cycle each.
- **Dependencies**: Task 5.4.
- **Flag**: rumours won't appear in the API/feed until Milestone 1.4
  (`rumour upsert + clustering`, currently open PR #10) merges — nothing
  yet turns `articles.extraction` into a `rumours` row. This roadmap makes
  the ingest/extract pipeline run reliably in production; it does not
  implement 1.4. Expect an empty deployed feed until then.

---

## Phase 6 — Frontend Deploy (Vercel)

### Task 6.1 — Fix the `API_BASE_URL` production footgun
- **Repo**: transfer-scout-web
- **Why**: confirmed — `src/lib/api.ts:6`:
  `const API_BASE_URL = process.env.API_BASE_URL ?? "http://localhost:8080";`
  silently falls back to localhost in production — a real risk on Vercel.
- **Scope**: change `src/lib/api.ts` to throw a clear error when
  `API_BASE_URL` is unset **and** `process.env.NODE_ENV === "production"`,
  checked inside `apiFetch` (not at module-eval time) so
  `src/lib/__tests__/api.test.ts` (mocks `global.fetch` directly) isn't
  broken by an eager import-time throw.
- **Acceptance criteria**: a production run with `API_BASE_URL` unset fails
  loudly and immediately instead of silently attempting `localhost:8080`;
  existing tests in `src/lib/__tests__/api.test.ts` still pass unmodified
  if possible.
- **Dependencies**: none; land before Task 6.2.

### Task 6.2 — Deploy to Vercel
- **Repo**: transfer-scout-web
- **Scope**: connect the GitHub repo to a new Vercel project; set
  `API_BASE_URL` in Vercel project settings (Production environment) to the
  Cloud Run API URL from Task 4.4; consider adding an explicit
  `"packageManager": "pnpm@<version>"` field to `package.json` (currently
  absent) so Vercel's inferred pnpm version matches local dev.
- **Acceptance criteria**: production Vercel URL loads the real feed
  against the real Cloud Run API; the next PR's preview deployment builds
  green with CI (Task 2.2).
- **Dependencies**: Tasks 4.4, 6.1.
- **Note (not a task)**: no CORS work is needed. Data fetching happens
  server-to-server (Next.js Server Component → Go API) — despite Vercel and
  Cloud Run being different domains, the browser never calls the API
  cross-origin today. Leave `internal/api/router.go` without CORS
  middleware; revisit only if a future Client Component calls the API
  directly from the browser.

---

## Phase 7 — Observability & Hardening (fast-follow after initial deploy)

### Task 7.1 — `internal/api` handler test coverage
- **Repo**: transfer-scout-api
- **Why**: confirmed — only `parseIntParam` has a unit test today; the
  actual handlers (`handleListRumours`, `handleGetRumour`) have none. Worth
  closing before trusting the public API in production.
- **Scope**: introduce a narrow `Store` interface (e.g. `ListRumours`,
  `GetRumourByID`, `Ping` from Task 4.2) in `internal/api` so `Server` can
  be built with a fake in tests — mirrors the fake-backed pattern already
  used in `internal/ingest`/`internal/extract`. `*store.Store` already
  satisfies it, so `cmd/api/main.go`'s construction call needs no change.
  Add table-driven tests: empty-list nil-slice-to-`[]` handling, pagination
  clamping through the real handler, 404 on unknown id, 400 on malformed
  UUID, 500-on-store-error.
- **Acceptance criteria**: `go test ./internal/api/...` exercises both
  handlers against fakes, no real Postgres required.
- **Dependencies**: Task 4.2 (adds `Ping` to the store surface — land
  together).

### Task 7.2 — Rate limiting & request hardening
- **Repo**: transfer-scout-api
- **Why**: no rate limiting or request-size limits exist. Auth is
  explicitly out of scope, but an unauthenticated, pay-per-request Cloud
  Run service in front of a free-tier Neon DB has real cost/availability
  exposure to scraping or accidental hammering — cheap insurance that
  doesn't conflict with staying public.
- **Scope**: in `internal/api/router.go`, add chi's `middleware.Timeout`
  (~10s), `middleware.RequestID`, `middleware.RealIP` (useful behind Cloud
  Run's proxy). Add `github.com/go-chi/httprate` as a per-IP limiter (e.g.
  60 req/min) scoped to the `/api/v1` group only — leave `/healthz`
  unlimited so Cloud Run's own health probes are never throttled. Skip
  request-body-size limiting (every endpoint is `GET`, no bodies); note it
  explicitly as future work for when write endpoints (predictions) exist,
  rather than adding dead code now.
- **Acceptance criteria**: hammering `/api/v1/rumours` past the threshold
  returns 429; `/healthz` unaffected; Task 7.1's tests still pass.
- **Dependencies**: Task 7.1 (avoid interface churn mid-flight).

### Task 7.3 (optional/stretch) — Error tracking
- **Repo**: cross-cutting
- **Why**: no error reporting anywhere — only `log/slog` to stdout. Not
  explicitly requested by the user, flagged as optional.
- **Scope**: if pursued, minimal Sentry integration — Go SDK in each Go
  `main.go`'s error paths, `@sentry/nextjs` wired into `error.tsx` +
  `instrumentation.ts`.
- **Dependencies**: Phases 4–6 (deploy first, see what actually breaks).

### Task 7.4 (optional/stretch) — CD automation
- **Repo**: cross-cutting
- **Why**: the CI decision covers lint/vet/build/test, not deploy — deploys
  in this roadmap are manual (`gcloud`, Vercel's own auto-deploy). Flagged
  so this isn't reinvented later.
- **Scope**: a `deploy.yml` on push to `main` running `gcloud builds submit`
  + `gcloud run deploy` (Vercel already auto-deploys via its GitHub
  integration — no extra workflow needed there). Use Workload Identity
  Federation, not a long-lived `GCP_SA_KEY`, as the GitHub secret.
- **Dependencies**: all of Phases 1–6.

---

## Phase 8 — Cross-Cutting Documentation

### Task 8.1 — Root-level orientation doc
- **Repo**: cross-cutting, new file at repo-pair root
  (`/Users/diogo/Desktop/projects/market-transfer-prj/README.md`)
- **Scope**: short doc summarizing both repos' purpose, linking to each
  repo's own `CLAUDE.md` (especially the new Deployment sections from Tasks
  3.1/4.3/5.5/6.2), and the overall architecture (Cloud Run Service + Jobs
  + Cloud Scheduler + Neon + Vercel). Note explicitly that there's no root
  git repo — nothing at this level should assume repo-wide git operations.
- **Acceptance criteria**: file exists, cross-links both repos' deployment
  docs accurately.
- **Dependencies**: do this last, after Tasks 3.1, 4.3, 5.5, 6.2 have their
  own docs in place.

---

## Phase 9 — Frontend Polish (lower priority, after deploy is live)

### Task 9.1 — Basic SEO
- **Repo**: transfer-scout-web
- **Why**: no OpenGraph/Twitter tags, no `robots.txt`/`sitemap.ts`.
- **Scope**: expand `metadata` in `src/app/layout.tsx` with `openGraph`/
  `twitter` fields; add `src/app/robots.ts` and `src/app/sitemap.ts`
  (Next.js App Router file-convention generators) referencing the real
  Vercel domain from Task 6.2.
- **Acceptance criteria**: `/robots.txt`/`/sitemap.xml` resolve on the
  deployed URL; a link-unfurl test shows a proper title/description card.
- **Dependencies**: Task 6.2 (need the real domain).

### Task 9.2 — Loading states
- **Repo**: transfer-scout-web
- **Why**: confirmed — no `loading.tsx` anywhere; both routes do live
  server-side fetches with no interim UI.
- **Scope**: add `src/app/loading.tsx` (feed skeleton) and
  `src/app/rumours/[id]/loading.tsx` (detail skeleton), styled consistent
  with `src/components/rumour-list.tsx`.
- **Acceptance criteria**: an artificially-delayed dev fetch shows the
  skeleton before real content streams in; existing tests unaffected.
- **Dependencies**: none functionally; sequence after Task 6.2.
