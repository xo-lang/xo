# std/sql: the in memory driver is a subset, and a Tx holds a pool permit

Status: Anticipated (2026-10-08).

Symptoms and causes:

- A query that works against Postgres fails in tests with `Syntax(...)`:
  the in memory driver (`sql.memory`, `testing.fake_db`) has no joins,
  grouping, subqueries, `RETURNING`, or `?` placeholders (only `$n`). Keep
  test schemas and statements inside the subset in spec/std.md 10.6, or test
  the real statements against a real driver.
- A program hangs after `db.begin()` on a failing path: a `Tx` holds one pool
  permit (and, for the in memory driver, the only writer permit) until
  `commit` or `rollback`. A second `begin` waits forever if the first was
  never finished. Always write `defer { tx.rollback().ignore_err("...") }`
  right after `begin` (rollback of a finished Tx is a no-op).
- `rollback()` returns a `Result`, so a bare `defer tx.rollback()` is XO0201;
  discard it with `.ignore_err("reason")`.
- A cancelled task gets `Canceled` from a query; inside a shielded `defer`
  cancellation is not reported, so cleanup queries still run.
- Reads outside a transaction do not see its writes until commit (read
  committed); reads inside see their own writes.
- The module functions `sql.int`, `sql.str`, `sql.bool`, `sql.float`,
  `sql.time` build parameters and share names with Row accessors; the
  accessors are methods (`row.int("id")`), the builders are functions.
