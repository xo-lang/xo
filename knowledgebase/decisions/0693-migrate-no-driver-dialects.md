# 0693: std/migrate has no dialect for a database std/sql has no driver for

Status: Accepted (owner, 2026-11-17, through the lead; written by Xo Lang 2; task #197)

## Context

Decision 0655 audits std/migrate against pressly/goose v3.27.3, and features Proven needs every
goose symbol in scripts/stdaudit/map.txt mapped, not planned, or closed. Goose names a dialect for
each database it supports. std/migrate's Dialect covers the databases std/sql reaches: Postgres and
MySQL (std/sql's drivers), SQLite (std/sqlite), sql.memory, and the databases that speak one of
those wire protocols (TiDB, Turso, Redshift, Aurora DSQL; task #190). Six goose dialects name a
database whose wire protocol std/sql has no driver for: ClickHouse, SQL Server, Spanner, StarRocks,
Vertica and YDB. A dialect is a migration table's SQL plus a lock; without a driver a program cannot
open the database, so a dialect for it could not run or be tested, and the six rows stayed `gap:`,
which kept migrate from features Proven.

## Decision

The six dialects are not planned: their map.txt rows read `not planned: design choice: decision
0693: ...`. A dialect returns, as a gap to close, when std/sql gains a driver for its database (a
new driver is its own task; the dialect then follows goose's SQL for it). TiDB, Redshift and Aurora
DSQL stay rows of their own (mapped to Dialect as task #190 lands them, gaps until then).

## Consequences

- stdcov counts the six rows as resolved, so migrate's features mark no longer waits on databases
  Xo cannot reach.
- Adding a std/sql driver (for example SQL Server's TDS or ClickHouse's native protocol) reopens the
  matching dialect row as a gap in the same change.

## Alternatives

- Ship the dialects' SQL without a driver: untestable code that no program can run, and a user
  could not tell it never ran.
- Keep the rows as gaps: migrate could not reach features Proven until six drivers exist, which is
  out of proportion to a migration table's SQL.
- Let a user's own Driver use a goose dialect: a custom dialect is goose's Store extension point,
  already not planned (`plug-in extension points`).
