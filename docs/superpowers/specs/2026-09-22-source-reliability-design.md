# Source reliability: outcome-based accuracy

Status: approved design. Implementation deliberately postponed — see
Prerequisites.
Date: 2026-09-22

## Problem

Transfer Scout ranks rumours by a `credibility` figure derived from
`sources.reliability_score`. That score is maintained by a single nudge in
`internal/cluster/cluster.go`:

```go
if justResolved {
    delta := +2
    if rumour.Status == models.StatusCollapsed { delta = -2 }
    NudgeSourceReliability(ctx, rumour.ID, delta)
}
```

```sql
UPDATE sources
SET reliability_score = LEAST(100, GREATEST(0, reliability_score + $2))
WHERE id IN (SELECT DISTINCT source_id FROM rumour_events WHERE rumour_id = $1)
```

It is already outcome-triggered — it fires when a rumour reaches `confirmed` or
`collapsed` — but it is too blunt to sell or to defend:

1. **No hit rate.** It is a mutable float, not attempts-versus-hits. We cannot
   say "this source is right 72% of the time", cannot explain a score, and
   cannot rank honestly.
2. **Volume accumulates score.** Report everything and the `+2`s add up.
3. **Timeliness is unrewarded.** Breaking a story first is worth the same as
   piling on once it is obvious.
4. **`rumour_events.confidence` is captured and never used.**
5. **No provenance.** There is no record of which rumours produced a score, so
   nothing can be audited or shown to a user.

The product intent is a user-facing analytics feature — the paid tier's hook is
"which sources are actually right". Numbers users see must be auditable and
explainable, which a tuned float is not.

## Decisions

| Question | Decision |
|---|---|
| Primary purpose | User-facing analytics (premium hook) |
| Headline metric | Hit rate on resolved rumours; unit of judgement is the rumour |
| Denominator | Explicitly resolved only (`confirmed` + `collapsed`); stale rumours ignored |
| Small samples | Minimum resolved count to be ranked; below it, show the raw fraction labelled insufficient |
| Existing float | Retired |

Architecture: **derive on read from `rumour_events`**, rather than materialising
a `source_outcomes` table or keeping incremental counters.

Rejected — materialised outcome table: faster reads and an explicit audit trail,
but a definition change means a migration plus a backfill plus a reconciliation
story. We have already accepted one known gap (below), so the definition *will*
change.

Rejected — incremental `attempts`/`hits` counters on `sources`: cheapest reads,
but it drifts, cannot be recomputed, and repeats the exact flaw being removed —
a number with no provenance.

Deriving cannot drift from the events it summarises, and yields explainability
for free: the same query without the aggregation is the list of rumours a source
was judged on.

## Design

### Accuracy derivation

A SQL view, created in the same migration that retires the float, so the
definition lives in one place and serves both the credibility join and the
future `/sources` endpoint:

```sql
CREATE VIEW source_accuracy AS
SELECT e.source_id,
       COUNT(DISTINCT r.id) FILTER (WHERE r.status = 'confirmed') AS hits,
       COUNT(DISTINCT r.id)                                       AS attempts
FROM rumour_events e
JOIN rumours r ON r.id = e.rumour_id
WHERE r.status IN ('confirmed', 'collapsed')
GROUP BY e.source_id;
```

`COUNT(DISTINCT r.id)` is load-bearing: a source publishing five articles about
one rumour must count once. Without it, volume inflates accuracy — the flaw
being removed.

Hit rate is `hits::float / attempts`, computed above SQL so the raw counts stay
available for display and for the threshold.

**No backfill is required.** The metric derives from `rumour_events`, which is
intact, so it is retroactively correct from the first day of collected data.

### Credibility

`credibility` keeps its name and its place on the rumour views, but changes
meaning: the average hit rate across the rumour's *ranked* sources, `NULL` when
none qualify (rendered by the existing `omitempty`).

It must be computed by joining `source_accuracy` **once per query**, not as a
correlated subquery per row as today's credibility is. The feed returns up to
100 rows and was just indexed for exactly this hot path; re-aggregating
`rumour_events` per row would undo that.

### Ranking threshold

`MIN_RESOLVED_FOR_RANK = 10`.

The threshold is **policy, never baked into the view**. The view returns raw
`hits` and `attempts` only.

For source listings the API applies it in Go. For `credibility` — which is
averaged inside the feed query — the threshold is **passed in as a bound query
parameter**, not hardcoded in SQL. So the value still lives in one place in Go
and tuning it never requires a migration, while the SQL can still exclude
unranked sources from the average. (Stating this explicitly because "policy
above SQL" and "credibility averaged in SQL" would otherwise contradict.)

Below the threshold: show the raw fraction (`"2 of 3"`) labelled as insufficient
data, exclude the source from rankings and from the credibility average.

### Breaking a collapse does not earn back the miss

When a rumour collapses, every source that reported it takes a miss — including
the outlet that later broke the news that it had collapsed. This is deliberate,
not a rough edge.

The unit of judgement is the rumour. A source that published "Player X to Club Y"
on a deal that died was wrong when it published, and reporting the collapse
afterwards is good journalism that does not retroactively make the original claim
correct. Crediting the later call would let a source launder a bad rumour by
covering its own retraction.

### Retiring the float

One migration:

- `CREATE VIEW source_accuracy` and `ALTER TABLE sources DROP COLUMN
  reliability_score`. Order is unconstrained: the view reads `rumour_events` and
  `rumours`, never the dropped column.

And in Go, delete:

- `NudgeSourceReliability` (`internal/store/rumours.go`) and its `cluster.go`
  caller, plus the `reliabilityNudge` constant and the `justResolved` nudge block
- `ReliabilityScore` from `models.Source`
- the column from `ListSources`' select in `internal/store/sources.go`

Nothing user-facing breaks: there is no `/sources` endpoint today, so
`reliability_score` is not publicly exposed. The only public derivative is
`credibility`, which is preserved by name.

The down migration can only restore the column with its `50.00` default, not the
accumulated values. This is accepted: those values are precisely the
untrustworthy number being replaced. The migration is therefore one-way in
practice.

## Prerequisites — why this is postponed

Implementation is deliberately deferred until there is enough resolved data.

`MIN_RESOLVED_FOR_RANK = 10` is likely unreachable today: the corpus is young and
most rumours never resolve at all. If no source clears ten resolved rumours, every
source falls below the threshold, so `credibility` would be `NULL` on every rumour
in the feed — strictly worse than the imperfect number shown today.

Before implementing, check the real distribution:

```sql
SELECT e.source_id, COUNT(DISTINCT r.id) AS resolved
FROM rumour_events e
JOIN rumours r ON r.id = e.rumour_id
WHERE r.status IN ('confirmed', 'collapsed')
GROUP BY e.source_id
ORDER BY resolved DESC;
```

Start phase 1 when several sources clear the threshold. If the data is close but
thin, lower the threshold rather than shipping a feed full of `NULL` credibility —
but do not drop it low enough to put a 1-of-1 source at the top of a leaderboard,
which is the failure this threshold exists to prevent.

## Phasing

1. **Backend metric.** The view, the credibility swap, the float retired. No new
   endpoints. Stands alone and de-risks the rest.
2. **`GET /sources` and the audit trail** — per-source `hits`/`attempts`/rate
   plus "the rumours you were judged on". The premium surface.
3. **Web leaderboard.**

## Testing

Integration tests against Docker Postgres, each shown failing first:

- a source with mixed outcomes reports the correct `hits`/`attempts`
- a source with several articles on one rumour counts that rumour **once**
- unresolved rumours are excluded from both numerator and denominator
- a source below the threshold is excluded from ranking and from credibility
- a rumour whose sources all fall below the threshold yields `NULL` credibility
- the feed query's plan still uses `idx_rumours_updated_at` after the
  credibility join is added (guards the hot path)

## Accepted limitations

1. **Stale rumours are invisible.** Most transfer rumours never resolve — a dead
   deal rarely gets a retraction. Counting only resolved rumours means a source
   can farm accuracy by publishing speculation that quietly evaporates, and the
   effect grows with the volume of junk published. Accepted deliberately for
   simplicity and fairness. Revisit by surfacing a separate *unresolved rate*
   per source rather than contaminating the headline number.
2. **Timeliness and `confidence` remain unused.** Both are recorded; neither
   feeds the metric. Scoop rate is a strong differentiator for a transfer
   product and is the most likely phase-4 addition.
