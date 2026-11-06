# 0534: SQLite driver for std/sql

Status: Accepted (owner, 2026-11-06, "go with your recommendations"), with three conditions:
fs effects, not net.dial (item 5); std/sql's portable error codes (item 8); the amalgamation
vendored with a hash check, its version in NOTICE and an update procedure (item 11). The
std/sql API change item 5 needs is with the owner (lead, 2026-11-06); std/sql is not changed
until that answer.

## Context

The owner queued SQLite, MySQL, Redis and zstd for std after 0.41.0. SQLite is the database
most small backends start on (Go: github.com/mattn/go-sqlite3, cgo, MIT; modernc.org/sqlite,
the C library translated to Go, BSD-3). Unlike Postgres (decision 0278), SQLite is not a
protocol: it is about 250,000 lines of C (the amalgamation, public domain) that reads and
writes the database file itself. Decision 0205 keeps the runtime and every program without
`use c` free of a C toolchain; decision 0273 ships a prebuilt C artifact (ThreadSanitizer's
runtime) per release target, linked by xo's own linkers. No std module uses `use c` yet.

The binder (spec 4.5, compiler/cli/bind/gen.xo) skips output pointers (`sqlite3 **` in
sqlite3_open_v2, `sqlite3_stmt **` in sqlite3_prepare_v3), destructor function pointers
(bind_text, bind_blob), and every pointer result but `const char *` (column_text is
`const unsigned char *`, column_blob `const void *`), so the library cannot be bound as is.

## Decision

1. **std/sqlite, a driver over the amalgamation through `use c`**, registered for
   `sql.open_driver`: `sql.open_driver(net, sqlite.driver(fs), "file:app.db?_busy_timeout=5000")`.
   Its own module, not inside std/sql, so std/sql stays free of C and builds on the Go and
   wasm backends; std/sqlite is XO1201 there (as `use c` is).
2. **Prebuilt per release target (route 2), stage 2.** The release builds sqlite3.c and a 60 line shim
   (xo_sqlite.c) into one object per target (darwin arm64/amd64, linux arm64/amd64,
   windows/amd64) under lib/xo/sqlite/, as 0273's runtime; users need no C compiler and
   `--targets` cross builds work for shipped targets. Stage 1 (now): `use c ... link
   "sqlite/sqlite3.c"`, built with the checkout's C compiler on first use, so development
   needs none of the release steps.
3. **The shim** folds output pointers into handle results (xo_sq_open, xo_sq_prepare), binds
   with SQLITE_TRANSIENT (Xo's Str and Bytes are released after the call), and returns text
   and blob columns as a pointer and length.
4. **A binder addition** (spec 4.5): a C function returning a struct of exactly
   `{const void *p; long long n}` by value binds as a `Bytes` result (copied; NULL is empty).
   Text columns use it too (a `const char *` result stops at NUL).
5. **Effects and capability (owner condition: fs, not net.dial).** std/sqlite's `use c` line
   claims `uses fs`: a local database file is file access. std/sql's driver interfaces
   declare `uses net.dial` only, so no driver that performs fs can implement them today
   (XO0701 on every method, checked 2026-11-06). The fix is an effect parameter on std/sql's
   driver types (core 6.2): `interface Driver[fx: Effects]`, `DriverConn[fx]`,
   `DriverCursor[fx]`, `struct DriverRows[fx]`, `Db[fx]`, `Tx[fx]`, `Rows[fx]`, and
   `open_driver[fx](net, d: Driver[fx], ...) -> Db[fx]`; Db, Tx and Rows methods perform
   `(net.dial, fx)`. Postgres and the in memory driver are `fx` empty. Code that writes `Db`
   with no effect argument gets an implicit effect variable (core 6.2), so existing code
   compiles unchanged, and a SQLite Db's fs reaches the functions that call it. A probe
   program on xo 0.41 (Conn[fx], Db[fx], a caller of plain `Db` declaring `uses net.dial`)
   checks and runs. **Gap:** the checker does not infer an interface's effect argument from
   the implementing struct's methods (a FileConn passed as `Conn` is `Conn[()]`, XO0701), so
   `sqlite.driver(fs)` is declared `-> sql.Driver[fs]`; inferring it is a separate checker
   change. The Fs given to `driver` roots and limits the file: a sub Fs roots the path (an
   absolute path or `..` is refused), a read only Fs opens SQLITE_OPEN_READONLY, a write only
   Fs is refused. Not `net.dial`: claiming it would mislead `xo vet` and capability review.
6. **DSN** as mattn's: `file:PATH?...`, `:memory:`, `_busy_timeout`, `_journal_mode`,
   `_foreign_keys`, `_synchronous`, `cache=shared`, `mode=ro|rw|rwc|memory`, so Go users' DSNs
   work; an unknown `_` key is Param (mattn ignores it, so a typo silently does nothing).
7. **Types** as mattn's: INTEGER Int, REAL Float, TEXT Str, BLOB Bytes, NULL Null. Binds: Flag
   as 0 or 1, At as RFC 3339 text, JsonText as text, Array refused. Placeholders `$N`, `?N`
   and `?` bind by position; named ones are Param. A TIMESTAMP or DATETIME column parsed into
   Time (mattn's rule) is stage 2.
8. **Errors (owner condition: portable codes).** std/sql's variants, as the memory and
   Postgres drivers report them: UNIQUE and PRIMARY KEY violations are `Constraint(code:
   "23505")`, FOREIGN KEY "23503", NOT NULL "23502", CHECK "23514", with SQLite's message;
   "no such table" is `NoTable`, a parse error `Syntax`; anything else `Server(code: the
   extended result code's name, e.g. "SQLITE_BUSY", msg: sqlite3_errmsg)`. Code that matches
   `Constraint(code: "23505")` works on all three drivers.
9. **Transactions:** BEGIN / COMMIT / ROLLBACK. The `SET TRANSACTION` std/sql sends after BEGIN
   is taken by the driver: READ ONLY becomes `PRAGMA query_only = ON` until COMMIT or ROLLBACK,
   and an isolation level is accepted and ignored (SQLite's transactions are serializable). No
   DriverConn change.
10. **copy_from:** a prepared INSERT run per row inside the caller's transaction (not the
    interface's "Driver" refusal), since bulk loading is SQLite's common use.

11. **Vendoring (owner condition).** std/native/sqlite holds sqlite3.c and sqlite3.h of the
    release amalgamation (about 9.1 MB and 0.7 MB in the repository, public domain), the shim,
    and VERSION with the zip's URL and SHA3-256. NOTICE names SQLite and its version. Update
    procedure: download `sqlite-amalgamation-NNNNNNN.zip` from sqlite.org, check its SHA3-256
    against the one sqlite.org publishes for that release, replace sqlite3.c and sqlite3.h,
    update VERSION and NOTICE in the same commit, rerun std/sqlite's tests, golden
    native_sqlite (its first line prints the version) and the differential corpus, and add a
    std/changes/sqlite.md entry naming the SQLite release.

## Solid on three axes (decision 0461)
- Go differential: one statement corpus through mattn/go-sqlite3 and std/sqlite: rows, types,
  error codes and texts; the corpus in internal/stdtest/testdata/corpus/sqlite.
- Fuzz: DSN parser, value conversion, prepared statement reuse.
- Timing row: bench/perf/sqlite (insert 100k in one transaction, point select by rowid and by
  index, a join) against mattn and modernc on linux/arm64.

## Consequences
- Two checker bugs a std `use c` module hit were fixed first (gotchas/std-use-c-binding-types.md,
  cmd/xo TestStdModuleUseC): the binding is declared with std's tree, and its paths resolve
  under the std directory.
- The std/sql effect parameter (item 5) changes std/sql's declared types; no source change is
  expected for code that writes `Db` without an effect argument.
- std's binding cache is std/native/.xo/cbind, inside the std tree: a read only std (a package
  manager's install) needs the release to ship the binding (stage 2, with item 2's objects).
- std's first C: the release grows by one object per target (about 1.5 MB each); the seed and
  release scripts build it; std-coverage.md gets the row.
- The binder gains a Bytes result kind, usable by any `use c` program.

## Alternatives
- `use c` of sqlite3.c at the user's build (route 1): needs cc per target for every user;
  first build compiles 9 MB of C. Kept as the checkout path only.
- Translate the C to Xo (modernc's route): pure Xo everywhere, wasm too; very large; parked
  as the long term path.
- A pure Xo engine reading the file format: smallest dependency, largest correctness risk.

## First experiments (2026-11-06)

- exp1 (route 2): sqlite3.c 3.54.1 compiled to one object (`-O2 -DSQLITE_THREADSAFE=2
  -DSQLITE_OMIT_LOAD_EXTENSION`) plus a shim, linked through `use c` with `xo build
  --backend=arm64`: on darwin/arm64 clang takes 7 s and the object is 1,522,808 bytes, on
  linux/arm64 gcc 14.2 takes 12 s and 1,383,384 bytes; both link and run, handle results bind
  as written, and a UNIQUE violation reaches Xo as extended code 2067
  (SQLITE_CONSTRAINT_UNIQUE).
- Stage 1 driver (2026-11-06, branch l2-sqlite): std/native/sqlite.xo over the vendored
  3.54.1; 4 module tests and golden native_sqlite pass on darwin/arm64 (binds, a blob with
  NUL bytes, 23505, 23503, 23502, NoTable, Syntax, named and missing placeholders, a rolled
  back and a committed transaction).
- exp2: blobs with NUL bytes round-trip through the binder's struct {pointer, length} Bytes
  result (item 4, on branch l2-bytesn with golden use_c_bytes; `int` binds as Int32, so the
  shim's constants are Int32).
