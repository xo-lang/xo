# std/sqlite changes

## 2026-11-18: fix: a writer beside busy writers gets its turn before its busy timeout

- Kind: fix
- Commit: this push
- Found by: task #229 (xw-1 #152's std/sql stress evidence): a cold writer beside 8 hot ones on one
  WAL file failed at its 2 s busy timeout in 9 of 10 runs on the Mac
- Go reference: github.com/mattn/go-sqlite3 v1.14.53 fails the same program in 4 of 20 runs (its
  busy handler sleeps in C); Xo now fails none
- Test: std/sqlite_test.xo "a writer beside busy writers gets its turn before its busy timeout"
  (failed before 3 of 3: SQLITE_BUSY "database is locked")
- Shipped in: not yet released

A writer that got SQLITE_BUSY napped and tried again, while one that had just finished a statement
stepped again at once and won nearly every release. A write outside a transaction now goes
through a write gate per database file in the process: a writer that got SQLITE_BUSY joins a
queue, and once the oldest waiter has waited 50 ms (and 50 ms have passed since the last such
turn) the next turn is its own. Between those turns writers step as before, so throughput is
unchanged (69,000 to 129,000 inserts in the same program, as before). Statements inside a
transaction, reads, and connections in other processes are not gated.

## 2026-11-17: fix: a statement's prepare waits out the busy timeout

- Kind: fix
- Commit: this push
- Found by: task #195: the std/sql stress evidence (xw-1 #152, host 3) failed with
  "INSERT 48000000: SQLITE_BUSY: database is locked" on a client's first statement
- Go reference: github.com/mattn/go-sqlite3 v1.14.53, whose sqlite3_busy_timeout handler also
  covers sqlite3_prepare_v2
- Test: std/sqlite_test.xo "a prepare waits out the busy timeout as a step does" (failed before:
  SQLITE_BUSY at once)
- Shipped in: not yet released

A new connection's first statement reads the schema while it is prepared, and gets SQLITE_BUSY
while another connection holds a write lock. Only sqlite3_step was retried, so the statement failed
at once whatever `_busy_timeout` said. The prepare is now retried as a step is: parked between tries
(1 ms growing to 16 ms) until the busy timeout has passed.

## 2026-11-16: feature: mattn's other DSN keys, named extended result codes, and the audit

- Kind: feature, fix
- Commit: this push
- Found by: task #109 (features to Proven): the decision 0655 symbol audit of mattn/go-sqlite3 v1.14.53
  (scripts/stdaudit/outside.txt, map.txt: 138 symbols and a behavior checklist)
- Go reference: github.com/mattn/go-sqlite3 v1.14.53 (MIT), sqlite3.go's DSN keys and error.go's codes
- Test: std/native/sqlite_test.xo "config reads mattn's other pragma keys and _txlock", "_txlock=exclusive
  begins transactions with BEGIN EXCLUSIVE...", "Server errors name the extended result code as
  sqlite3.h does" (failed before: SQLITE_1811)
- Shipped in: not yet released

The DSN takes mattn's `_txlock` (deferred, immediate, exclusive: the BEGIN a transaction starts with),
`_auto_vacuum` (`_vacuum`), `_cache_size`, `_case_sensitive_like` (`_cslike`), `_defer_foreign_keys`
(`_defer_fk`), `_ignore_check_constraints`, `_locking_mode` (`_locking`), `_query_only`,
`_recursive_triggers` (`_rt`), `_secure_delete` and `_writable_schema`, and accepts `_loc` (a Time has
no zone, and mattn's `_loc` only labels the same instant); they were Param errors. Fix: a Server error's
code names every extended result code mattn names (`SQLITE_CONSTRAINT_TRIGGER`, `SQLITE_IOERR_READ`), not
`SQLITE_1811`.
Departures from mattn, in the module doc and map.txt: no online backup (`VACUUM INTO` is a one shot snapshot,
not an incremental copy) and no Deserialize into memory (through a file).

## 2026-11-16: test: a differential corpus against mattn/go-sqlite3, and the timing row

- Kind: test
- Commit: this push
- Found by: task #109 (features to Proven), the SQLite driver row's remaining items
- Go reference: github.com/mattn/go-sqlite3 v1.14.53 (MIT), the same SQLite 3.54.1 amalgamation
- Test: internal/stdtest/testdata/corpus/sqlite (scripts/stdcorpus/sqliteoracle: 180 cases, 30 for the
  spec's mappings and 150 seeded tables of mixed storage classes and declared affinities, each run by
  mattn through database/sql on an in-memory database: rows with their storage classes and float bits,
  exec counts and last insert ids, and errors as std/sql's variants); TestStdCorpora sqlite on arm64 and
  LLVM (the Go backend refuses `use c`, XO1201); every case matched on the first run
- Timing: std/evidence/sqlite/perf.md, linux/arm64 host 1, 9 rounds: insert 0.869, point 0.801, scan 0.694
  of mattn's time
- Shipped in: not yet released

## 2026-11-10: feature: freebsd/arm64 and freebsd/amd64

- Kind: feature
- Commit: this push
- Found by: decision 0573 `use c` stage 4
- Go reference: none
- Test: internal/llvm goldens native_sqlite and native_sqlite_file (a database file in WAL and rollback journal modes, reopened, integrity_check) in the FreeBSD 14.5 VMs, arm64 and amd64
- Timing: none
- Shipped in: not yet released

std/sqlite builds and runs for FreeBSD, which links no C library: the C names sqlite's unix VFS calls come from Xo's runtime (std/native/llrt/cabi_cbind.xo: __error, getenv, access, fchmod, fchown, fstat, lstat, fsync, ftruncate, lseek, geteuid, gettimeofday, nanosleep, utimes) and from the C the build adds beside a binding (compiler/xo/fbsdstdio.xo: pthread's mutexes on _umtx_op, fabs, sysconf, localtime and the dl functions). SQLite's `localtime` modifier is UTC there (no time zone data), and extensions cannot be loaded (a static program).

## 2026-11-07: feature: windows/amd64 and prebuilt objects

- Kind: feature
- Commit: this commit
- Found by: decision 0534 stage 2 (owner-accepted addendum)
- Go reference: none
- Test: cmd/xo TestStdSQLiteCrossBuilds (windows/amd64 with no clang or sysroot from a prebuilt object);
  the native_sqlite golden built and run natively on Windows
- Timing: unchanged (same amalgamation and flags; the object is prebuilt instead of compiled per build)

std/sqlite builds for windows/amd64 with xo's own PE linker: SQLite's few C library calls go to the
no-CRT shim (strcmp, memchr, malloc, free, abort, _localtime64_s, ...). A release ships the SQLite object
compiled ahead of time for each platform, used when its key matches the sources, so a program using
std/sqlite builds without a C compiler, and the std binding's symbols no longer depend on where the
toolchain is installed.

## 2026-11-06: feature: std/sqlite, a SQLite driver for sql.open_driver

- Kind: feature
- Commit: this commit
- Found by: owner-approved roadmap item (Xo Lead, 2026-11-06), decision 0534
- Go reference: none in Go's standard library; github.com/mattn/go-sqlite3 (MIT) for the DSN, the type
  mapping and the differential corpus
- Test: std/native/sqlite_test.xo (DSN keys, an unknown key, a memory database's binds, blob round trip,
  constraint and missing table errors, transactions)
- Timing: pending (bench/perf/sqlite against mattn and modernc on linux/arm64)

New module over the vendored SQLite 3.54.1 amalgamation (std/native/sqlite, public domain, SHA3-256 of the
release zip in its VERSION) through `use c`. Native backends only.
