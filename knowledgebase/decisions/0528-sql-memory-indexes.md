# 0528: std/sql memory driver: indexes on UNIQUE columns and writes in place

Status: Accepted (owner, 2026-11-05: "Go with your recommendations", through the lead). Built in the same push.

## Context

(Note 2026-11-06: the Go twin's GET /notes walked every id issued so far under its lock (O(creates) per list) before 472c82653, so Go relative rows measured before it are inflated (gotchas/twin-cost-grows-with-history.md; rerun in stability-load-2026-11-05.md's 2026-11-06 addendum). With the twin fixed, the map store variant's 4000/s p50 is 0.682 of Go's: the Xo against Xo ratios here stand, the Go relative ones do not.)

Decision 0520's load check found this (bench/results/stability-load-2026-11-05.md). At 4000/s offered,
stability/notes' p50 was 1.47 times its Go twin's and its p99.9 1.62 times, while it used less CPU per request.
At 1000/s and at saturation Xo was ahead. Spinning idle workers did not change it (XO_SPIN_ROUNDS=16: 229 us
against 234). A variant of notes with the twin's store (a Map behind a Mutex, users cached by token digest) cut
Xo's CPU per request at 4000/s from 83.5 us to 28 us, beat the twin on p50 (0.39) on the same loaded host, and
ended the gap.

std/native/sql.xo's memory driver (`sql.memory`) does O(rows) work on every statement:

- SELECT, UPDATE and DELETE filter by a full scan of the table (mem_select: `for row in t.rows { where_ok }`).
  A primary key lookup (`WHERE id = $1`) visits every row.
- INSERT and UPDATE check each UNIQUE column by scanning every row (check_row).
- Every write copies the whole table. Db.exec runs the statement on `var work = st.tables` so that a failing
  statement leaves the database as it was. The statement then mutates a Table it read with table_for, while the
  map still holds it, so Xo's copy on write copies the table's row list (and DELETE rebuilds it besides). In the
  CPU profile this is `dup sql.SqlValue` (5.8% flat), on top of mem_select's 15% and EvalEnv.eval's 11%
  (cumulative).
- All of this runs under the Db's one lock, so the time a request holds it grows with the table. A p50 that rises
  with the offered rate (148 us at 1000/s, 234 us at 4000/s, while Go stayed at 172 and 159) fits a serialized
  section that is held longer.

Every statement is also lexed and parsed on each call, but that does not show in the profile's top 30.

## Decision

1. **Validate, then apply, in place.** Each single-statement write computes and checks everything first: the new
   rows of an INSERT, the replacement rows of an UPDATE, the positions a DELETE removes. Every error (a coercion,
   NOT NULL, UNIQUE, a bad parameter) happens in that phase. Only then does it apply the changes to the table,
   in a Map.update whose closure cannot fail, after the plan function that read the table has returned, so the
   table has one holder and nothing is copied (see Implementation). Db.exec
   drops the `work` copy for a single statement; a text of several statements (a schema) keeps today's copy, so
   it stays all or nothing. A transaction's snapshot still shares the tables, so the first write while one is
   open copies once, as now: transaction semantics are unchanged.
2. **Deletes leave tombstones.** A deleted row's slot becomes an empty row (a table always has at least one
   column, so an empty row is never a real one). Scans skip empty rows. When tombstones are more than half the
   slots and more than 64, the table is compacted and its indexes are rebuilt. Rows keep their insertion order, so
   a SELECT without ORDER BY returns the same order as before.
3. **An equality index per UNIQUE column** (SERIAL columns are UNIQUE). Each is a map from the stored value to its
   slot, one map for Integer values and one for Text, kept by insert, update, delete and compaction. A column
   holding a value of any other type has no index (scans, as now). check_row's UNIQUE check becomes one lookup.
4. **A WHERE fast path.** When a SELECT, UPDATE or DELETE has a WHERE that is `col = v` (either side), or an AND
   with such a conjunct, on an indexed column, with v a parameter or literal of exactly that index's type (Integer
   or Text, not Null), it visits only the slot the index names. It still evaluates the whole WHERE on that row, so
   the result is the same as the scan's. Anything else scans.

Implementation (2026-11-05). Each write is a plan function (insert_plan, update_plan, delete_plan: parse,
evaluate, check, return a Plan of the rows or slots) and an apply step in `tables.update`. Three things kept the
first build copying the table on every write; each was found by timing, then bisecting with never-run calls:

- Map.remove and a put back do not leave a value unique: the hybrid Map in its log form keeps each value it held
  in its value log (decision 0291), so the removed table still shared its rows. Map.update with a closure that
  cannot fail changes the value in place (map_update_take).
- A CREATE or DROP ran on a copy of the map (`var work = tables`) to keep a failed end of statement from changing
  anything; a written copy is what turns a Map into the log form. Both now check the statement's end before their
  one write and run on the map itself.
- `coerce`'s array branch (`items.map` with a closure) and `compact` (`rows.filter` with a closure) made every
  Db.exec body cancelable: a call through a function value is a cancellation point, and a cancelable
  `Mutex.with` body keeps the value it was given (core 5.4), so its first write copied the table. Both are loops
  now (gotchas/with-body-cancelable-through-closure-call.md).

Not in this decision: a parsed statement cache, which the profile does not call for; indexes on non-UNIQUE
columns; range or ORDER BY use of an index.

## Consequences

- Lookups and writes by key are O(1) in the table's size; scans of other WHEREs are as before, minus the copy.
- Results do not change: the same rows in the same order, the same errors with the same codes. The existing sql
  goldens (gen sql_basic, sql_decode, sql_tx, sql_wire_names, test_sql; llvm native_sql, sql_conv) and std's sql
  tests must pass unchanged. New xo tests: get, update and delete by id on 10,000 rows; a UNIQUE violation on
  insert and on update still reported and leaving the table unchanged; a failing multi-row INSERT leaves no row;
  the order of an unordered SELECT after deletes, inserts and a compaction; an UPDATE of an indexed column moves
  its index entry; a transaction's snapshot unchanged by writes outside it.
- Timing (decision 0276's row against main): a micro in bench/perf (get by id, and insert then delete, at 1,000 and
  10,000 rows), and harness load at 4000/s, notes against its twin.
- std/changes/sql.md entry. No Go reference for the engine: Go's database/sql has no engine (SQLite's rowid and
  unique indexes are the model).
- Extension (2026-11-06, Xo Lang 2, the lead agreeing): a non UNIQUE column gets an index only through
  `CREATE INDEX [IF NOT EXISTS] [name] ON table (column)` (PostgreSQL's syntax, one column; `DROP INDEX`), kept as
  each value's slots in ascending order so the rows come in the scan's order; the planner prefers a UNIQUE equality
  and falls back to an indexed one. Indexing every INT and TEXT column instead would make every insert pay for
  columns no query filters on (notes.body). The Go backend's driver takes the statements with the same errors and
  scans. Found by the sat64 profile of stability/notes on the sql store (`WHERE owner = $1`, 23% of CPU in
  mem_select). A value held by more than a quarter of the live rows is scanned instead of visited through its
  slot list. The notes service does not declare the index: its harness load has one owner, so the index selected
  every row and cost 0.82 of the scan's sat64 throughput and 12 MB of peak RSS (host 1, 2026-11-06); its timing row
  is bench/perf/sqlmem's many-owner select.

## Alternatives

- **An undo log** recording each change and replaying it backwards on an error. Needed only if errors could happen
  after the first change; validating first removes them, with less code.
- **A Map of rows keyed by rowid** instead of a list with tombstones. Map keeps insertion order, but iterating
  it costs more per row than a list scan, and every scan would change.
- **A statement cache only.** Cheaper to build, but the profile shows scans and copies, not parsing.
- **Leave it and document that the memory driver is for tests.** Rejected: the notes service uses it as its store,
  as examples/notes-service does, and the corpus apps are meant to be realistic programs.
