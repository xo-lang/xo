# 0534: SQLite driver for std/sql

Status: Accepted (owner, 2026-11-06, "go with your recommendations"), with three conditions:
fs effects, not net.dial (item 5); std/sql's portable error codes (item 8); the amalgamation
vendored with a hash check, its version in THIRD_PARTY_NOTICES (the owner's "NOTICE") and an update procedure (item 11). The
std/sql API change item 5 needs (the effect parameter on Driver, DriverConn, DriverRows,
DriverCursor, Db, Tx and Rows) is approved too (owner, 2026-11-06, "go with your recommendations",
through the lead) and lands with std/sqlite.

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
    and VERSION with the zip's URL and SHA3-256. THIRD_PARTY_NOTICES names SQLite and its version. Update
    procedure: download `sqlite-amalgamation-NNNNNNN.zip` from sqlite.org, check its SHA3-256
    against the one sqlite.org publishes for that release, replace sqlite3.c and sqlite3.h,
    update VERSION and THIRD_PARTY_NOTICES in the same commit, rerun std/sqlite's tests, golden
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
- The std/sql effect parameter (item 5) changes std/sql's declared types. Code that writes `Db`
  without an effect argument still checks (an implicit effect variable), except where a Db is
  captured in a closure whose type fixes its effects: that function and the ones passing the Db
  to it write `sql.Db[()]` (stability/notes: 3 of its 14 signatures; an XO0701 fix-it for
  `xo fix` follows).
- The Go backend: std/sqlite is in std/go_compiled.txt, so a Go backend build loads
  std/native/sqlite.xo and its `use c` is XO1201 (cmd/xo TestStdSQLiteGoBackend); stdgen skips a
  module whose native file has `use c`, so std/rt has no Go for it. std/rt's std_sigs_sql.go
  checks Db, Tx and Rows's methods too: std_sigs (compiler/gen stdtypes.xo) skips a function
  or receiver only for a type parameter that is not an effect variable, since an effect
  parameter has no Go type (which also brought in 8 effect generic functions of http, testing,
  websocket and the prelude that were never checked).
- std's binding cache is std/native/.xo/cbind, inside the std tree: a read only std (a package
  manager's install) needs the release to ship the binding (stage 2, with item 2's objects).
- std's first C: the release grows by one object per target (about 1.5 MB each); the seed and
  release scripts build it; std-coverage.md gets the row.
- The binder gains a Bytes result kind, usable by any `use c` program.

- std's store (decision 0388) does not hold std/sqlite: its `use c` made the store build link with cc and fail (2026-11-06, main red at 7b1cbe8da; gotchas/std-store-build-use-c-module.md), so the std roots skip any std module with a `use c` line, and a program that imports sqlite builds it as its own part. The vendored sqlite3.c is on .gitleaks.toml's path allowlist (4 generic-api-key false positives; its bytes are pinned by TestSQLiteVendored).

## Alternatives
- `use c` of sqlite3.c at the user's build (route 1): needs cc per target for every user;
  first build compiles 9 MB of C. Kept as the checkout path only.
- Translate the C to Xo (modernc's route): pure Xo everywhere, wasm too; very large; parked
  as the long term path.
- A pure Xo engine reading the file format: smallest dependency, largest correctness risk.
- A default effect argument (`struct Db[fx: Effects = ()]`, a bare `sql.Db` meaning `Db[()]`;
  lead and owner, 2026-11-06, not taken): Postgres and in memory users would never annotate,
  but every std/sqlite user would write `sql.Db[fs]` wherever a Db is kept, so the case the
  parameter exists for becomes the one that needs annotating, and generic code over drivers
  writes `Db[fx]` everywhere. Today's rule annotates only a Db captured in a closure with fixed
  effects; the XO0701 fix-it writes that annotation.

## Addendum (2026-11-07, Proposed): stage 2's prebuilt objects, and how windows/amd64 links one

Status: Proposed (Xo Lang 1, through the lead, for the owner).

**Context.** Stage 2 (item 2) was never built: std/sqlite compiles sqlite3.c from source
with the host's C compiler on every target. A windows/amd64 build compiles it with clang
against the MSVC C library and the Windows SDK, which a Mac or Linux host has only as
XO_WINSYSROOT (xwin's sysroot), so hourly.sh's cross step failed on native_sqlite with
"'stdlib.h' file not found" from f04a8b4ca until 05a606d61 made it a named limit
(gotchas/std-use-c-cross-sample-miss.md). A windows/amd64 program with no `use c` links
with Xo's PE linker and no C runtime (decision 0254 6b step 4); a `use c` program falls back
to clang and MSVC's libraries. Item 2 says users need no C compiler; on Windows they need
more than that today.

**Probe (2026-11-07, the Mac, clang 23.1.3, xwin's headers; ~/xo-lead/notes/lang1/sqwin-probe.txt).**
sqlite3.c 3.54.1 and xo_sqlite.c compiled for x86_64-pc-windows-msvc, then `llvm-nm -u`:

- `-O2 -DSQLITE_THREADSAFE=2 -DSQLITE_OMIT_LOAD_EXTENSION` (stage 1's flags): KERNEL32
  functions, plus the C runtime's malloc, free, realloc, _msize, _beginthreadex,
  _endthreadex, _localtime64_s, __C_specific_handler, __chkstk, _fltused, and memchr,
  memcmp, memcpy, memmove, memset, strchr, strcmp, strcspn, strlen, strncmp, strrchr and
  strspn.
- Adding `-DSQLITE_WIN32_MALLOC=1 -DSQLITE_MAX_WORKER_THREADS=0 -DSQLITE_OMIT_LOCALTIME
  -DSQLITE_OMIT_SEH -mno-stack-arg-probe` leaves only:
  - KERNEL32: AreFileApisANSI, CloseHandle, CreateFileMappingW, CreateFileW, DeleteFileW,
    FlushFileBuffers, FlushViewOfFile, FormatMessageW, GetCurrentProcessId, GetDriveTypeW,
    GetFileAttributesExW, GetFileAttributesW, GetFileSizeEx, GetFullPathNameW,
    GetLastError, GetProcessHeap, GetSystemInfo, GetSystemTimeAsFileTime, GetTempPathW,
    GetTickCount64, HeapAlloc, HeapCompact, HeapCreate, HeapDestroy, HeapFree, HeapReAlloc,
    HeapSize, HeapValidate, LocalFree, LockFileEx, MapViewOfFile, MultiByteToWideChar,
    OutputDebugStringA, QueryPerformanceCounter, ReadFile, SetEndOfFile, SetFilePointerEx,
    Sleep, UnlockFileEx, UnmapViewOfFile, WaitForSingleObject, WaitForSingleObjectEx,
    WideCharToMultiByte, WriteFile; and through `__imp_`: AcquireSRWLockExclusive,
    DeleteCriticalSection, EnterCriticalSection, GetOverlappedResult,
    InitializeCriticalSection, InitializeSRWLock, LeaveCriticalSection,
    ReleaseSRWLockExclusive, TryAcquireSRWLockExclusive, TryEnterCriticalSection.
  - the C library: memchr, memcmp, memcpy, memmove, memset, strchr, strcmp, strcspn,
    strlen, strncmp, strrchr, strspn; and _fltused (a data symbol).
  - The object is 1,641,389 bytes.

std/native/llrt/cabi_wincrt.xo already exports memcpy, memmove, memset, memcmp and strlen
for the no C runtime link; the other seven are small ports for llrt/libcstr.xo.

**Proposal.**

1. **windows/amd64 links the prebuilt object with Xo's PE linker, no C runtime.** The
   release compiles sqlite3.c and xo_sqlite.c with the probe's flags, except two:
   - keep `localtime` (an SQL date modifier users reach): cabi_wincrt exports
     `_localtime64_s` (GetTimeZoneInformation and SystemTimeToTzSpecificLocalTime) instead
     of `-DSQLITE_OMIT_LOCALTIME`;
   - keep stack probes: llrt provides `__chkstk` (a page by page probe, as MSVC's) instead
     of `-mno-stack-arg-probe`, so a frame larger than a page still meets the guard page.

   The release writes the object to lib/xo/sqlite/windows-amd64/sqlite.obj with a .sha256,
   as decision 0273's race_windows_amd64.syso. The PE linker resolves its imports through
   compiler/macho/peimports.xo (regenerated by scripts/win-import-table.py with the
   functions above). cabi_wincrt gains the seven string functions and `_fltused`. A
   windows/amd64 build that uses std/sqlite then needs no C compiler, no MSVC library and no
   SDK, from any host, so `--targets` cross builds work as item 2 promises.
2. **darwin and linux ship an object too** (lib/xo/sqlite/GOOS-ARCH/sqlite.o, built by the
   release on each target: the Mac for both darwin targets, the offload hosts for linux). It
   saves the 7 to 12 s compile of the first build. The link stays the host cc's, as for any
   `use c` program; a cc free link there is outside this addendum.
3. **The binding ships with the objects** (the Consequences' read only std): the release
   writes std/native/.xo/cbind's sqlite binding into lib/xo/std, so binding needs no C
   header parse at the user's build.
4. **Lookup.** std/sqlite's `use c` link source resolves to the prebuilt object of the
   build's target when lib/xo/sqlite/GOOS-ARCH/ has one beside the running xo and its hash
   matches. Otherwise it compiles sqlite3.c from source as now (a checkout, stage 1). A
   windows/amd64 target with neither keeps 05a606d61's message.
5. **Tests.** cmd/xo TestStdSQLiteCrossBuilds expects windows/amd64 to link once a
   prebuilt is present. The windows golden (native_sqlite) runs on the amd64 host's Windows
   when that is scripted, and as a single threaded Wine smoke until then (Wine is not a
   gate). The release checks each object's undefined symbols against peimports.xo and
   cabi_wincrt's exports, so a SQLite update that needs a new C function fails the release,
   not a user's link.

**Costs.**
- `SQLITE_OMIT_SEH` drops SQLite's guard around memory mapped reads, so an I/O error on a
  mapped page faults instead of returning SQLITE_IOERR. Memory mapping is off unless a
  program sets `PRAGMA mmap_size`.
- `SQLITE_MAX_WORKER_THREADS=0` turns off the sorter's helper threads (a large ORDER BY
  without an index sorts on one thread).
- `SQLITE_WIN32_MALLOC` puts SQLite's memory on a Win32 heap of its own, not Xo's.
- Each release builds 5 objects (about 1.5 MB each).

**Alternatives.**
- Ship MSVC's and the SDK's import libraries: their license does not allow redistribution.
- Ship sqlite3.dll beside every program: no longer one file, and a DLL search path to
  secure.
- mingw-w64 objects: needs mingw's C runtime import libraries, so a C runtime returns to a
  program that has none.
- Keep XO_WINSYSROOT as the only way: users install clang and xwin, the opposite of item 2.
- Translate SQLite to Xo: still the long term path (Alternatives above), far larger.

### Implementation (2026-11-07, Xo Lang 1)

What the addendum needed beyond its text, found while building it:

- **The `use c` adapter is C too.** The binding's generated adapter
  (std/native/.xo/cbind/c_xo_sqlite_*.c) is compiled per build beside
  sqlite3.c and xo_sqlite.c, and calls malloc, free and abort. The object is
  one compile of the three (scripts/sqlite-prebuilt.sh writes a unit that
  includes them; xo_sqlite.h gained an include guard for it), and
  cabi_wincrt.xo defines malloc and free (the process heap) and abort
  (ExitProcess(3), as MSVC's).
- **Item 3 needed a different binding key.** cimport.xo named a binding by
  its header's and link files' absolute paths, so the symbol prefix
  (`c_xo_sqlite_<8 hex>_`) and with it every adapter symbol differed per
  checkout, and the checker re-bound the header with clang on every run. A
  std header's binding is now named by `bind.std_key`: the paths relative to
  std plus the header's SHA-256. Its files are the same on every machine, a
  binding that is there is not bound again (the key covers the header's
  bytes), its adapter includes the header relative to the module (the
  build's copy names this machine's path), and its decl names no path. The
  release ships it in lib/xo/std/native/.xo/cbind, so a std/sqlite build
  needs no clang.
- **Lookup.** compiler/xo cffi.xo `sqlite_prebuilt`: lib/xo/sqlite/GOOS-ARCH/
  sqlite.o (sqlite.obj) beside std whose `.key` equals `sqlite_key` (SHA-256
  of the target, sqlite3.c, xo_sqlite.c, xo_sqlite.h and the adapter); a
  windows/amd64 build whose every `use c` binding has one is a no C runtime
  build (link.xo `nolibc`), and the PE link takes the object.
- **`__chkstk`** is emitted with the other Windows stubs
  (compiler/arm64 x86win.xo `w64_chkstk`, compiler-rt's probe loop) in a no C
  runtime build. `_fltused` needs nothing: clang lists it with no relocation.
- **`_localtime64_s`** converts by SystemTimeToTzSpecificLocalTime with the
  tm as its scratch (cabi_wincrt.xo allocates nothing).
- **Item 5.** sqlite-prebuilt.sh fails when the windows object needs a symbol
  that is not in peimports.xo, cabi_wincrt.xo or the runtime (xo_x_*).
- Linux objects are not built yet (the release runs on the Mac); a linux
  build compiles sqlite3.c on first use as before.

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
