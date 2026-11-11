# 0609: column type metadata in std/sql (Rows.column_types)

Status: Accepted (owner, 2026-11-11; Xo Lang 3)

## Context

The std audit of 2026-11-11, gap 25: `Rows.columns()` gives the names
only. Go's `sql.Rows.ColumnTypes` (from the driver's
`RowsColumnTypeDatabaseTypeName`, `Length`, `Nullable`,
`PrecisionScale` and `ScanType`) is used by generic tools: CSV and JSON
exporters, admin pages and schema checks that format a value by its
column's type, and migration tools that compare a query's shape with a
struct. Each driver already reads this information. Postgres's
RowDescription carries each field's type OID and modifier (pgwire.Field),
sqlite has `sqlite3_column_decltype`, and the memory driver has the
table's schema. None of it reaches the caller.

## Decision

1. `struct ColumnType` in std/sql:

   ```
   struct ColumnType {
     name: Str
     database_type: Str = ""       // DatabaseTypeName, upper case ("INT4", "VARCHAR", "NUMERIC"); "" when unknown
     length: Int? = None           // Length: varchar(n), bytea and text as Go's drivers report them
     precision: Int? = None        // DecimalSize
     scale: Int? = None
     nullable: Bool? = None        // Nullable; None when the driver cannot tell
     kind: Str = ""                // the SqlValue variant the column decodes to ("Integer", "Text", ...); ScanType
   }
   fn (r: Rows) column_types() -> List[ColumnType]
   ```

   It is also on `Row` (`row.column_types()`), as `columns` is.
2. Drivers report it through a new defaulted field on `DriverRows`:
   `column_types: List[ColumnType] = []`. A driver that leaves it empty,
   including every driver written before this decision, gives
   `ColumnType{name}` per column (Go returns "" and `ok = false` for an
   unsupported method). No interface changes, so existing drivers keep
   compiling.
3. The built-in drivers fill it as their Go counterparts do:
   - Postgres: pgx's names for the type OIDs (`INT4`, `TEXT`,
     `VARCHAR`, `NUMERIC`, `TIMESTAMPTZ`, ...); length from the
     modifier for varchar and bpchar (`modifier - 4`), unbounded types
     as pgx reports them; precision and scale for numeric
     (`(modifier - 4) >> 16` and `& 0xffff`); nullable None, because
     the protocol does not carry it (pgx returns `ok = false` too).
   - sqlite: `sqlite3_column_decltype` upper-cased, as mattn/go-sqlite3
     does, and nullable None.
   - memory: the schema's declared type and its `NOT NULL`.
4. Tests: Postgres against pgx's `ColumnTypes` on the pg-diff setup
   (scripts/pg-diff.sh) for a table with every supported type; sqlite
   against mattn/go-sqlite3 in its corpus; memory with goldens.

## Consequences

- Generic exporters and admin tools can format by column type without
  parsing the query.
- `DriverRows` grows one field. Building it costs a little per result
  (one ColumnType per column, from data the driver already has), not
  per row.
- The Go backend's memory driver fills the same fields.

## Alternatives

- Methods on a driver interface (Go's optional `RowsColumnType*`
  interfaces): Xo has no optional interface check by type assertion,
  and a required method would break every existing driver.
- Exposing pgwire.Field directly: Postgres specific, and it leaves
  sqlite and memory without an answer.

## Progress

- 2026-11-11 (Xo Lang 3): built as decided, on both backends. The Go
  backend's RegisterSqlDriver backends report types through an optional
  QueryTypes / QueryTypesTk method, so the SqlBackend interface is
  unchanged. Postgres matches pgx stdlib on scripts/pg-diff.sh (a types
  line per SELECT, pgx v5.12.0's type names), including its quirks: an
  unbounded varchar has length -5 and an unconstrained numeric precision
  65535, scale 65531. sqlite reports the declared type only, as
  decided; mattn's ColumnTypeNullable says (true, true) for every
  column, which std/sql leaves as None since sqlite cannot tell.
