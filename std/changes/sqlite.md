# std/sqlite changes

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
