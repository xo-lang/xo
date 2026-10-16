# Where `xo test` time goes (2026-10-15)

The development Mac (Apple silicon), one run at a time through the heavy
lock, xo built from main at 6cf57f58. Each package was tested three
times on each backend with fresh `XO_BUILD_DIR`, `XO_BACKEND_CACHE_DIR`
and `XO_GOCACHE`: cold, warm (nothing changed), and edit. The edit was
a comment appended to the first file in the profile, and a new test
appended in the "after" runs. `--phase-times` gives the phases in ms;
wall is `/usr/bin/time -p` real. Wall minus phases is running the tests
(one process per test, decision 0235's subject) plus the driver.

## Profile before (base 6cf57f58)

| Package | Backend | Cold s | Warm s | Edit s | Largest phases (cold, then warm) |
|---|---|---|---|---|---|
| selfhost/prune (11 tests) | arm64 | 2.38 | 1.74 | 1.46 | check 601, insert_rc 377, assemble 262; warm check 625 |
| | llvm | 3.16 | 2.89 | 2.90 | cc 1866 every run |
| | go | 4.85 | 0.23 | 0.15 | |
| examples/notes-service (16) | arm64 | 18.85 | 3.81 | 3.78 | emit 12565, add_landings 1454, assemble 1332; warm add_landings 1403 |
| | llvm | 42.08 | 43.15 | 42.73 | cc 37998 every run |
| | go | 6.11 | 0.62 | 0.60 | |
| selfhost/lexer (20) | arm64 | 6.07 | 4.36 | 4.35 | phases 2162 cold, 1037 warm: about 3.3 s running tests |
| | llvm | 6.80 | 6.60 | 6.51 | cc 2327 every run |
| | go | 5.00 | 0.28 | 0.28 | |
| selfhost/check (15) | arm64, llvm | | | | not buildable without a GC (XO1201, MemFs read_link) |
| | go | 11.12 | 1.57 | 1.56 | |

Findings:

1. LLVM's link (`cc`) is the largest cost, and it is paid on every run:
   test binaries were built at -O2 with full LTO, and the binary cache
   was off by default.
2. arm64's first build is mostly emission (notes-service 12.6 s), which
   the backend cache covers after that. add_landings costs 1.4 s on
   every notes-service run, 100 times its cost on the other packages;
   that is the next arm64 target (Xo lang 5 took it).
3. Every native run pays check (0.5 s), insert_rc, fn_keys and ssa
   again, about 1 to 3.4 s warm.
4. Running the tests: selfhost/lexer's 20 processes take about 3.3 s.
   That is decision 0235's batching.
5. The Go backend's warm runs are cached whole (0.15 to 1.6 s).

## LLVM link time by level (examples/notes-service)

| `XO_LLVM_OPT` | LTO | cc cold ms | wall cold s | wall warm s (binary cached) |
|---|---|---|---|---|
| O0 | none | 5361 | 12.14 | 6.12 |
| O1 | full | 35308 | 39.64 | 3.81 |
| O1 | none | 28989 | 33.27 | 3.78 |
| O2 | full | 38669 | 42.78 | 3.81 |
| O2 | none | 32836 | 36.95 | 3.77 |

## After decision 0236 (LLVM tests at -O0, the test binary cache on)

| Package | Backend | Cold s | Warm s | Edit (new test) s |
|---|---|---|---|---|
| examples/notes-service | llvm | 11.94 | 6.35 | 11.43 |
| | arm64 | 19.02 | 4.02 | 4.47 |
| selfhost/lexer | llvm | 4.86 | 4.32 | 4.82 |
| | arm64 | 5.18 | 4.58 | 4.77 |
| selfhost/prune | llvm | 1.66 | 1.12 | 1.53 |
| | arm64 | 1.87 | 1.32 | 1.49 |

## The run cache (decision 0236 item 3)

A warm run with nothing changed finds the stored binary and test list
(phase `run_cache`, 28 to 99 ms for the key and the input hashes) and
only runs the tests. The machine was loaded (load average 22 to 29,
other sessions' builds), so the lexer row was measured again alone.

| Package | Backend | Warm before (0236 items 1-2) s | Warm with the run cache s |
|---|---|---|---|
| examples/notes-service | llvm | 6.35 | 2.59 to 2.76 |
| | arm64 | 4.02 | 1.51 to 1.62 |
| selfhost/lexer | llvm | 4.32 | 3.41 to 3.58 (alone) |
| | arm64 | 4.58 | 3.56 to 3.84 |
| selfhost/prune | llvm | 1.12 | 0.12 to 0.25 |
| | arm64 | 1.32 | 0.10 to 0.21 |

What is left of a warm run is running the tests, one process each
(selfhost/lexer's 20 tests about 3.3 s): decision 0235's batching.
With decision 0235's batching on main as well, examples/notes-service
on arm64 runs warm in 0.30 s (`run_cache` 26 ms), cold 18.18 s.
A miss costs the key (15 to 150 ms; about 1.2 s under the load above).
Invalidation, checked by hand and by cmd/xo TestNoGCRunCache: an edit,
a new file, a changed xo.mod, an XO_ variable, and an edit to a std
file each miss; `-run` and the other run only flags hit.

## Known issue (fixed 2026-10-15): selfhost/check had no native test binary

`xo test --backend=arm64` (or llvm) of selfhost/check fails before any
code is generated: `locate.xo:136:11: error XO1201: not available
without a garbage collector (--no-gc) yet: the method Fs.read_link of a
test's MemFs (support is planned)`. Its tests run only on the Go
backend (11.1 s cold, 1.6 s warm) until MemFs gains read_link natively.

Fixed: std/native/testing.xo gives the native MemFs symbolic links
(memfs_symlink, memfs_read_link, and stat, exists, read, and read_text
following them as std/rt's MemFs does). `xo test -v selfhost/check`
prints the same report on Go, arm64, and LLVM (15 passed), and cmd/xo
TestNoGCTests now compares it.

With `XO_BIN_CACHE` unset (the default directory), selfhost/prune on
LLVM took 1.92 s and then 1.10 s.

The scripts are prof.sh, opt.sh and after.sh, run through the heavy
lock: `xo test --phase-times --backend=B .` in each package, with
fresh cache directories.
