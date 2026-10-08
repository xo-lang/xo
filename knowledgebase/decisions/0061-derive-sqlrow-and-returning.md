# 0061: `derive SqlRow`, typed queries, and RETURNING in the memory driver

Status: Accepted (2026-10-08)

## Context
notes-service mapped rows to structs by hand (gap 6 in
`bench/results/real-app-2026-10-08.md`): `row_note` read five columns by
name, and every new column touched it and the `INSERT`. `insert_note`
rebuilt the stored note from its arguments because the in memory driver
had no `RETURNING`. The gap proposed `row.decode[T: Json]()` (columns as
JSON keys) or a `derive Row`.

## Decision
- New derive **`SqlRow`** with a prelude marker interface `SqlRow {}`
  (like `Json`, every derivable name is an interface, core 2.4). Allowed on
  structs without type parameters whose fields are `Int`, `Float`, `Str`,
  `Bool`, `Time`, or an `Option` of one; anything else is XO0404 (field
  type) or XO0407 (enum, generic struct) at the derive. The generated Go
  method `XoSqlRowDec` reads each field from the column of the same name
  through `rt.SqlRowField`, which applies the accessor rules: missing
  column `NoColumn(name)`, wrong type or NULL into a non Option
  `Mismatch(column, msg)`, NULL into an Option `None`. Extra columns are
  ignored; field defaults do not apply.
- API (std.md 10.4): `row.decode[T: SqlRow]() -> T ! SqlErr`, and on `Db`
  and `Tx` `query_as[T]` (`List[T]`) and `query_one_as[T]` (`T`, `NoRows`
  when empty), with the parameters of `query`.
- The in memory driver accepts `RETURNING` (the `SELECT` items except
  `COUNT(*)`) on `INSERT`, `UPDATE`, and `DELETE`. Such a write runs
  through `query` / `query_one` (as with Postgres drivers); through `exec`
  its rows are dropped. `query` now runs on a private copy of the table
  map and publishes it on success, so a failed write through `query`
  changes nothing.

## Consequences
- notes-service: `row_note` is gone, `insert_note` is one
  `query_one_as[Note]("INSERT ... RETURNING *", ...)`, and `User` decodes
  with `row.decode[User]()`.
- `SqlRow` is now a predeclared (protected) name, like `Json`.
- Real drivers must return rows for `RETURNING` through `Query`; the
  `SqlBackend` interface did not change.
- Decoding is per column name lookup (linear in the columns); fine for the
  row widths of services. A cached column index per query is possible later.

## Alternatives
- `derive Row` (the name in the request): the derive name is also a
  prelude interface (core 2.4), so `Row` would become a protected name and
  would read as the `sql.Row` handle in std/sql's own declarations. Chosen
  `SqlRow`, which also says what it is for.
- `row.decode[T: Json]()` through JSON: no new derive, but it converts every
  row to JSON and back, maps `Time` through RFC 3339 text, reports errors as
  JSON paths instead of columns, and silently applies field defaults for
  missing columns. Rejected.
- `rows.decode[T]()` on `List[Row]`: methods can be declared only in the
  module of the receiver type (core 3.4), and `List` is a prelude type.
  `query_as` covers the common case without the intermediate list.
- `RETURNING` through `exec` with rows on `ExecResult`: a second way to read
  rows, unlike every SQL driver API. Rejected.
