# Test speed: `xo test` against `go test` (2026-10-21)

Owner-approved measurement (2026-10-21): how long a test cycle takes
for a std module whose Go test suite Xo has ported, against Go's own
package, cold, warm, and after a one-line edit, natively on arm64 and on
LLVM. Raw rows: [test-speed-vs-go-2026-10-21.csv](test-speed-vs-go-2026-10-21.csv)
(510 rows: side, module, backend, scenario, repetition, phase, wall,
user, sys, peak RSS).

## Method

- **Tree.** main at 41108904 plus the then-unpushed std differentials
  (csvpar, pathgo, archgo; all on main since: d99a3f57, 98e4292f,
  004dd2d4), xo 0.23.0 built from that tree by
  `go run ./scripts/xobuild`. Go 1.26.1.
- **Modules and tests.** Xo: `xo test --std-module=native/M` with M's
  std test file, Go's cases generated into it: path (3,549 cases, 1
  test), csv (3,061 cases, 2 tests), regexp (2,558 cases, 2 tests),
  json (3,059 cases, 1 test), archive (395 cases, 4 tests). Go: Go's own
  test binaries for the matching packages: path (9 tests),
  encoding/csv (3), regexp (44), encoding/json (89), and for archive the
  three packages archive/tar (43), archive/zip (52), compress/gzip (15),
  summed. **The cases differ**: Xo's files hold Go's results on Go's test
  inputs and random ones, while Go's suites also run benchmarks-free
  stress tests (regexp's exhaustive tests, the archive packages' large
  files), so the test-run column compares suites, not identical work.
  The build columns compare like with like (one test program per
  module).
- **Scenarios**, per repetition:
  - cold: empty user caches. Xo under a fresh `HOME` (every xo cache is
    under `$HOME/Library/Caches/xo` on macOS: build directories, the
    back-end, front-end, and binary caches); Go under a fresh `GOCACHE`
    (so the std packages the test imports are compiled too);
  - warm: the same caches again, nothing changed;
  - edit: a comment line added at the top of the module's source (Xo:
    std/native/M.xo; Go: the package's main file through
    `go test -overlay`, GOROOT untouched), then built.
- **Build and run split.** Go: `go test -c` (build) then the binary
  (run, `-test.count=1`). Xo: `xo test --run=NO_SUCH_TEST` (the build
  and an empty run) against the full `xo test`; the run is the warm full
  test minus the warm build.
- **Figures**: median of 5 repetitions; wall seconds with CPU seconds
  (user + sys) in parentheses; peak RSS of the timed process
  (`/usr/bin/time -l`): for Go that is the `go` command only, not its
  compile and link children, so Go's real peak is higher than shown;
  xo builds in process (clang is a child on LLVM).
- **Machine**: Apple M4 Pro (12 cores), 24 GB, macOS 27.0.1, other
  agents' jobs running: load average 25.2 at the start (08:19) and 14.1
  at the end (08:35), memory 80 to 82% free. All runs `nice`d. Run as a
  `HEAVY_CLASS=bulk` job.

## Results

| Module | Side | Cold build | Warm build | Edit build | Test run | Peak RSS (cold build) |
|---|---|---|---|---|---|---|
| path | xo arm64 | 4.22 (4.5) | 0.49 (0.6) | 0.94 (0.9) | 0.01 (0.0) | 2086 MB |
| path | xo llvm | 3.10 (2.8) | 0.63 (0.6) | 2.63 (2.6) | 0.00 (0.0) | 751 MB |
| path | go test | 2.44 (11.7) | 0.07 (0.3) | 0.42 (1.3) | 0.00 (0.0) | 310 MB |
| csv | xo arm64 | 2.79 (3.6) | 0.48 (0.6) | 0.80 (0.9) | 0.00 (0.0) | 1824 MB |
| csv | xo llvm | 3.51 (3.1) | 0.60 (0.7) | 2.60 (2.7) | 0.05 (0.0) | 768 MB |
| csv | go test | 4.43 (12.9) | 0.16 (0.2) | 0.41 (0.6) | 0.01 (0.0) | 315 MB |
| regexp | xo arm64 | 2.84 (3.5) | 0.72 (0.7) | 1.13 (1.2) | 0.00 (0.0) | 1121 MB |
| regexp | xo llvm | 3.63 (3.1) | 0.60 (0.6) | 3.17 (2.6) | 0.03 (0.1) | 734 MB |
| regexp | go test | 4.01 (13.3) | 0.13 (0.3) | 0.52 (0.9) | 4.67 (4.8) | 304 MB |
| json | xo arm64 | 3.85 (3.9) | 0.54 (0.6) | 0.98 (1.0) | 0.08 (0.1) | 1853 MB |
| json | xo llvm | 3.71 (3.2) | 0.72 (0.7) | 2.57 (2.5) | 0.02 (0.0) | 711 MB |
| json | go test | 6.71 (24.7) | 0.20 (0.5) | 0.90 (2.0) | 0.33 (0.4) | 321 MB |
| archive | xo arm64 | 4.22 (3.4) | 0.79 (0.8) | 1.62 (1.6) | 0.05 (0.1) | 769 MB |
| archive | xo llvm | 5.75 (3.7) | 0.86 (0.7) | 3.66 (2.7) | 0.40 (0.3) | 680 MB |
| archive | go test | 8.30 (25.2) | 0.40 (1.1) | 0.94 (2.0) | 4.19 (9.2) | 314 MB |

In short:

- **Cold**: Xo is level with Go or faster in wall time (2.8 to 5.8 s
  against 2.4 to 8.3 s) and uses far less CPU (2.8 to 4.5 s against 11.7
  to 25.2 s), because Go compiles every std package the test imports
  from source while xo links a prebuilt runtime.
- **Warm, nothing changed**: Xo 0.48 to 0.86 s against Go's 0.07 to
  0.40 s: 2 to 7 times slower.
- **Edit**: native arm64 0.80 to 1.62 s against Go's 0.41 to 0.94 s (1.7
  to 2.2 times); LLVM 2.57 to 3.66 s (4 to 6 times Go).
- **Test run**: Xo's ported cases run in 0.00 to 0.40 s; Go's fuller
  suites take up to 4.7 s (regexp's exhaustive tests, archive's large
  files), which this comparison does not hold against Xo.
- **Memory**: a cold native arm64 build peaks at 0.8 to 2.1 GB (2.1 GB
  for path's 3,549-case file), LLVM at 0.7 GB.

## The three biggest causes of the gaps

Phase times of json's warm and edit builds (`xo test --phase-times`,
one run after the measurement, a warm cache):

1. **LLVM edits wait on clang.** The edit build spends `phase cc`
   1,883 ms of about 2.6 s in clang compiling the regenerated IR of the
   test program, against 31 ms when nothing changed. The native arm64
   path has per-function caches down to the object (`cache functions
   498/500 reused`), so it pays 96 ms to assemble. Next experiment: split
   the test program's IR per module (XO_LLVM_SPLIT as xo1's own build
   does) so an edit recompiles only the edited module's part, and
   measure `phase cc` on the same edit.
2. **Any edit to a file rebuilds every body of its module.** The arm64
   edit's largest phase is `ssa` (337 to 665 ms, 95 ms warm). The
   front-end cache keys each function body with the module's interface
   closure, its files with the function bodies cut out (decision 0220
   addendum stage 2), so a comment line changes that hash. A comment
   added at the top of std/native/json.xo and one added at the bottom
   both gave `cache fe 0/1188 bodies` (one run each, warm cache). Next
   experiment: hash the closure from the declarations' signatures and
   types only (no comments, no positions), repeat both edits, and expect
   1,187 or 1,188 hits.
3. **Every warm build checks again.** `phase check` is 300 to 410 ms of
   a 0.5 to 0.7 s warm build on both backends, every time, while Go
   reuses the compiled package from its cache. xo's module check cache
   (`XO_CHK_CACHE`, decision 0220 addendum stage 2 step 2) is opt in and
   covers `xo build` and `xo run`, not `xo test`. Next experiment: turn
   it on for `xo test` of an unchanged module (`XO_CHK_CACHE=auto`) and
   measure the warm build; target under 0.2 s.

Also seen: the cold native build's peak of 2.1 GB for path (a 128 KB
test file of string constants) against 0.75 GB on LLVM; a profile of
that build (`XO_PROF=heap`) is the next step there. One interactive
build of the path test at load average about 25, before the measurement,
reported `phase write` at 40.5 s; none of the 50 timed path builds came
near that, so it is recorded as a load outlier, not a result.
