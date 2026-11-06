# 0534: SQLite driver for std/sql

Status: Proposed (Xo Lang 2, 2026-11-06; owner-approved item, the owner accepts the API).

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

## Decision (proposed)

1. **std/sqlite, a driver over the amalgamation through `use c`**, registered for
   `sql.open_driver`: `sql.open_driver(net, sqlite.driver(fs), "file:app.db?_busy_timeout=5000")`.
   Its own module, not inside std/sql, so std/sql stays free of C and builds on the Go and
   wasm backends; std/sqlite is XO1201 there (as `use c` is).
2. **Prebuilt per release target (route 2).** The release builds sqlite3.c and a 60 line shim
   (xo_sqlite.c) into one object per target (darwin arm64/amd64, linux arm64/amd64,
   windows/amd64) under lib/xo/sqlite/, as 0273's runtime; users need no C compiler and
   `--targets` cross builds work for shipped targets. A checkout without the object builds it
   with cc on first use (the `use c` path), so development needs none of the release steps.
3. **The shim** folds output pointers into handle results (xo_sq_open, xo_sq_prepare), binds
   with SQLITE_TRANSIENT (Xo's Str and Bytes are released after the call), and returns text
   and blob columns as a pointer and length.
4. **A binder addition** (spec 4.5): a C function returning a struct of exactly
   `{const void *p; long long n}` by value binds as a `Bytes` result (copied; NULL is empty).
   Text columns use it too (a `const char *` result stops at NUL).
5. **Capability:** driver(fs) takes an unattenuated Fs and refuses an attenuated one (SQLite
   opens the files itself; std/sql refuses an attenuated Net for the same reason); calls
   perform `ffi`, vouched by std as `uses fs`.
6. **DSN** as mattn's: `file:PATH?...`, `:memory:`, `_busy_timeout`, `_journal_mode`,
   `_foreign_keys`, `_synchronous`, `cache=shared`, `mode=ro|rw|rwc|memory`, so Go users' DSNs
   work; unknown keys are errors (mattn ignores them; ADR question).
7. **Types** as mattn's: INTEGER Int, REAL Float, TEXT Str, BLOB Bytes, NULL Null; a TIMESTAMP
   or DATETIME declared column parses into Time in Row.time (mattn's rule).
8. **Errors:** SqlErr.Server(code: the extended result code's name, e.g.
   "SQLITE_CONSTRAINT_UNIQUE", msg: sqlite3_errmsg), so a unique violation is told apart.
9. **Transactions:** BEGIN / COMMIT / ROLLBACK; std/sql's `SET TRANSACTION` after BEGIN for a
   non default isolation needs a driver hook (SQLite: read_only via `PRAGMA query_only`,
   isolation always serializable): a small DriverConn addition (`begin_opts`) or the driver
   ignores the statement (ADR question).
10. **copy_from:** a prepared INSERT run per row inside the caller's transaction (not the
    interface's "Driver" refusal), since bulk loading is SQLite's common use.

## Solid on three axes (decision 0461)
- Go differential: one statement corpus through mattn/go-sqlite3 and std/sqlite: rows, types,
  error codes and texts; the corpus in internal/stdtest/testdata/corpus/sqlite.
- Fuzz: DSN parser, value conversion, prepared statement reuse.
- Timing row: bench/perf/sqlite (insert 100k in one transaction, point select by rowid and by
  index, a join) against mattn and modernc on linux/arm64.

## Consequences
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
- exp2: blobs with NUL bytes round-trip through the binder's struct {pointer, length} Bytes
  result (item 4, on branch l2-bytesn with golden use_c_bytes; `int` binds as Int32, so the
  shim's constants are Int32).
