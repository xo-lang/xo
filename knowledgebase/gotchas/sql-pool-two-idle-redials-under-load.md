# A loaded std/ui Host on Postgres runs out of local ports: sql.open keeps 2 idle connections

Status: Observed (2026-11-17, std/ui users over several instances, decision 0686)

## Symptom

Four std/ui instances on `ui.sql_store` and `ui.pg_bus` against one
Postgres on the Mac, 2,000 simulated users: syncs failed with 503
`std/ui: store: connection: dial tcp 127.0.0.1:55495: can't assign
requested address`, while 200 users passed with every message stored.

## Cause

`sql.open` (and `sql.open_driver`) default to `max_open: 10, max_idle: 2`,
Go's database/sql defaults. A Host under load holds up to max_open
connections at once, and each one beyond the 2 idle slots is closed when
it is returned, so most queries dial a new connection. Each closed
connection leaves a local port in TIME_WAIT until the ephemeral range is
used up. Go behaves the same with its defaults.

## Fix

Open the database for a Host with `max_idle` equal to `max_open`:
`sql.open(net, "postgres", dsn, max_open: 16, max_idle: 16)`. The
`sql_store` and `pg_bus` docs say so. A load harness checks the TIME_WAIT
count (`netstat -an | grep -c TIME_WAIT`) before trusting its numbers, as
for the HTTP client pool
([http-client-pool-two-idle-per-host.md](http-client-pool-two-idle-per-host.md)).
