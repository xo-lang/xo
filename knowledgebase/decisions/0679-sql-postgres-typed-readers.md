# 0679: Typed readers for Postgres types in std/sql (task #113)

Status: Proposed (Xo Lang 3, 2026-11-16). The item list is #110's
backlog, which the owner approved as task #113 (2026-11-16: "port sql's
real Postgres types first"); this records the API shapes.

## Context

std/sql reads Postgres results in the text format and maps each column
to a `SqlValue` by its type OID (decision 0278). Types without a
mapping arrive as `Text`, so a program can read them with `Row.str` but
must parse them itself. pgx v5's pgtype decodes them into Go values
(UUID, Date, Time, Interval, netip.Prefix, net.HardwareAddr, Hstore,
Range[T], Numeric, composites). Task #110's triage counted 300 open
stdaudit rows for these types (sql 75 agent hours).

## Decision

1. A typed reader per type on `Row`, each with an `_opt` form for NULL,
   in the style of `Row.time`: the column's text is parsed by Postgres's
   output format (ISO DateStyle, the default IntervalStyle `postgres`),
   and a column of another kind fails with the existing mismatch error.
   A parameter goes as a `SqlValue` made by a constructor that writes
   the Postgres text form, so no new `SqlValue` variant is needed for
   these (the server parses the text by the parameter's type).
2. The types, each against pgx's pgtype for the same column:
   - uuid: `Row.uuid -> uuid.Uuid`, `sql.uuid(u)`.
   - date: `Row.date -> time.Date`; `infinity` and `-infinity` fail
     with a mismatch, and `Row.date_bound -> sql.Bound[time.Date]`
     (`Finite(v)`, `Infinity`, `NegInfinity`) reads them (pgx's
     InfinityModifier); the same `Bound` for timestamps
     (`Row.time_bound`). `sql.date(d)`.
   - time of day: `Row.time_of_day -> time.ClockTime` (time; timetz
     keeps its offset in `Row.str` only, as pgx's Time has no zone).
   - interval: `Row.interval -> sql.Interval{months, days, micros}`
     (pgx's Interval), with `Interval.duration() -> Duration?` when it
     has no months or days. `sql.interval(i)`.
   - inet and cidr: `Row.prefix -> netip.Prefix` (pgx scans both into
     netip.Prefix; an inet without a mask is a full-length prefix).
   - macaddr, macaddr8: `Row.mac -> Bytes` (6 or 8 bytes), shared with
     the prelude MAC type when that lands.
   - oid, xid, cid, xid8: already `Integer`; mapped to `Row.int` with a
     test.
   - hstore: `Row.hstore -> Map[Str, Str?]`, `sql.hstore(m)`.
   - ranges and multiranges: `sql.Range[T]` (lower, upper, each
     `Bound` with inclusive flags, empty) read by
     `Row.range_int`, `range_time`, `range_date`, `range_num`, and
     multiranges as `List[Range[T]]`.
   - numeric: an exact decimal; its representation (std/big's Rat or a
     decimal text type) is its own decision before it lands.
   - composites and LoadType: `Row.items` on a record column, and
     `Db.load_type(name)` registering a composite's or enum array's
     element OIDs for decoding (pgx's Conn.LoadType).
   - large objects, named arguments, COPY TO: their own items in
     #113's order, with their shapes recorded here when they land.
3. Each type lands with cases in scripts/pgdiff/cases.txt: the
   reference (scripts/pgdiff/gopgx) scans the column with pgx's pgtype
   and prints a canonical form, the Xo side reads it with the typed
   reader and prints the same form, on Postgres 14 and 17.

## Consequences

- No `SqlValue` variant is added, so matches on `SqlValue` keep
  compiling; `Row.value` still gives `Text` for these columns.
- std/sql uses std/uuid, std/netip and std/time's civil types (none of
  them uses std/sql).
- The open sql stdaudit rows close item by item; each push maps its
  pgtype rows in map.txt.

## Alternatives

- New `SqlValue` variants per type (`Uuid`, `Date`, ...): breaks every
  exhaustive match on `SqlValue` and grows the enum by about 15
  variants; the typed readers give the same values without that.
- Binary result format and pgx's codec registry: not planned (decision
  0655 rule 8, design choice by decision 0278).

## Implementation

- 2026-11-16 (Xo Lang 3): uuid, date, time of day, interval, inet and
  cidr: std/native/sql.xo and std/rt/sqltypes.go (the same parsers;
  std/uuid is rt hosted, std/rt_hosted.txt, so the Go backend returns
  its Uuid), golden sql_pg_types, pg-diff TYPED cases equal to pgx on
  Postgres 14 and 17. Still open in this group: the array OIDs (a typed
  list reader), timetz, the pgtype JSON methods of UUID and Date, and
  infinity (`Bound`).
- 2026-11-16 (Xo Lang 3): `Bound[T]` with `date_bound` and
  `time_bound`, `mac`, `hstore`, and xid, cid, xid8 decoded as Integer
  (std/native/llrt/pg.xo); golden and pg-diff cases as above (78
  statements equal to pgx).
