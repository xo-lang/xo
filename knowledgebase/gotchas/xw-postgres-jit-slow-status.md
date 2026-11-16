# xw status took a second: Postgres JIT compiled a small query

Status: Observed 2026-11-15 (xw-1).

## Symptom

`xw status`, `xw check` and the web view's `/` page took 0.85 to 2.4 s
while `xw queue` and `xw alerts` took 0.01 s. The tables were small
(events 2,800 rows, no bloat).

## Cause

The `ev` view's `who` column has correlated subqueries, so the planner's
cost estimate (about 300,000) passed `jit_above_cost` (100,000) and
Postgres JIT-compiled the query on every call: `EXPLAIN ANALYZE` showed
`JIT: ... Total 618.174 ms` of a 640 ms query; the scan node's startup
time carried it. With `SET jit = off` the query took 32 ms.

Two guesses came first and were wrong: one `ps` per open run, and
`build_activity`'s eight queries per session. Both changes stayed
(activity went to 50 ms), but neither moved `xw status`.

## Fix

Workstream migration 18 sets `jit = off` for xw's database (new
connections; restart `xw serve`). `xw status` takes 0.2 s.

## Avoid

Time each endpoint (`curl -w %{time_total}`) and run `EXPLAIN ANALYZE` on
the slow query before changing code; read the plan's JIT line when a
scan of a small table has a large startup time.
