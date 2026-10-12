# Self hosted checker coverage, 2026-10-12

Decision 0177. What kept the self hosted checker (`selfhost/check`)
from covering every repository run and every diagnostic of the Go
checker (internal/check), so that retiring the Go front end (decision
0150 as amended on 2026-10-11) is not blocked by it.

## Before

From the module loading section of selfhost-checker-2026-10-10.md (go,
arm64, and LLVM builds): 626 runs, 361 accepted and 249 rejected only
with compared diagnostics, all identical (610); 16 not compared:

- 15 with `use go` (13 internal/gen goldens and 2 replays): the Xo
  loader marked them unsupported;
- examples/realtime-control with `use c`. Since decision 0152 the `use
  c` runs (it, and internal/selfhost/testdata/ssa/stage6c and
  stage6c_skipped added with it) load, but on the Go backend both
  checkers reject the line with XO1201, a code the comparison did not
  list, so they counted as rejected for other reasons.

Diagnostics of internal/check with no port: XO0802, XO1101, XO1102,
XO1315, XO1316, XO1317 (from internal/mod). Ported but not compared:
XO1104, XO1105, XO1201.

## What changed

- `use go`: the declaration file internal/gobind cached
  (`<module root>/.xo/gobind`, or `$XO_GOBIND_DIR`), with gobind Load's
  staleness rules; missing or stale makes the run unsupported.
- Ported: XO0802 (escape.xo), XO1102 (goimport.xo), XO1315 (load.xo),
  XO1316 and `vouch` lines with their XO1301 (vouch.xo), XO1317 and the
  xo.lock `effects` lines (modfile.xo).
- Compared: XO0802, XO1102, XO1104, XO1105, XO1201, XO1315, XO1316,
  XO1317; the runs with `use c` again as native builds (`--native`).
- New fixtures: internal/selfhost/testdata/escape (12 runs, XO0802) and
  internal/selfhost/testdata/hatches (checked in a copy by
  TestSelfhostCheckerHatches: a dependency with `use go` in its own
  module cache, a main module without the vouch and one with it).

## After

`TestSelfhostChecker`, go build of the Xo checker (merge tier; the
arm64 and LLVM builds run in the full tier):

| Go checker result | runs | Xo checker matches |
|---|---|---|
| accepted: identical names, types, sigs, diags, full | 398 | 398 |
| rejected only with compared diagnostics: identical | 287 | 287 |
| rejected for other reasons (not compared) | 0 | |
| unsupported by the Xo checker | 0 | |
| total | 685 | 685 |

The 685 runs are the 626 of 2026-10-10, the 12 escape fixtures, and
runs other work added since. Of the 18 interop runs: the 15 `use go`
runs are accepted and identical; examples/realtime-control, stage6c,
and stage6c_skipped are rejected with XO1201 (and XO1105 for the
skipped C function) and identical. Checked as native builds, the 3
`use c` runs: 2 accepted and 1 rejected only with compared diagnostics
(XO1105), all identical. Diagnostics of the matching rejected runs
include 12 XO0802, 4 XO1201, 4 XO1315 (bench/paper modules with
`edition 2026`), and 1 XO1105.

`TestSelfhostCheckerHatches`: 2 runs, both identical: hatch rejected
with XO1316 (with the stale vouch's note), XO1301 (a malformed vouch),
XO1317, XO1315, two XO1102, and XO0110; hatchok accepted.

`TestSelfhostCheckerModuleMutations` (seed 20261011): 400 mutations,
1,709 runs, 353 accepted and 1,356 rejected only with compared
diagnostics, all identical, 0 unsupported (89 XO1315 among them).

Runs still not compared, by reason: none in the repository. A run is
unsupported only when its `use go` or `use c` binding is not in the
cache (the Xo checker does not run `go list` or clang) or a required
module version must be fetched; XO1101 (a Go package that cannot be
bound) is therefore not compared.

## Found on the way

1. internal/gobind `Load` kept the Go name at the end of a skip reason
   (` (strconv.FormatComplex)`) that `Bind` leaves out, so XO1102 read
   differently on the first check of a fresh directory and after
   (gotchas/gobind-skip-reason-load-vs-bind.md). Fixed; TestMapping
   compares the two.
2. A cached dependency with `use go` checked as its own run binds into
   its own tree and changes its content hash (XO1304 for its consumers,
   depending on run order; gotchas/gobind-cache-in-checked-dependency.md).
   The hatches fixture lives outside the repository walk for that reason.
3. internal/selfhost/testdata/modules/app/xo.lock was stale since the
   cache modules' xo.mod files were rewritten to `edition preview`
   (decision 0155): its 4 runs were rejected with XO1304 (28
   diagnostics) instead of reaching the deny rules and ceilings they
   test. Rewritten with `xo mod tidy` (now with effect lines).
4. Two internal/gobind tests expected handle names that decision 0172's
   prelude types now rename (`Cmd` is `ExecCmd`, `os.File` is `OsFile`);
   updated.

## Other gaps of the self hosted front end, from the results files

| component | still skipped or unsupported | count | why |
|---|---|---|---|
| lexer, parser | nothing | 0 | byte identical on every repository file |
| checker | XO1101, binding new packages | 0 runs in the repository | the binders (gobind's `go list`, cbind's clang) stay in Go (decisions 0152, 0177) |
| checker | XO0900 holes | not compared by design | decision 0141; builds stop on holes in both |
| SSA builder | runs the Go builder also refuses | 15 runs (llvm results), 80 test binary functions (ssa results, stage 7) | the Go builder reports the construct unsupported too, so nothing to compare |
| SSA builder | `use go` runs | not built natively | `use go` is Go backend only (decision 0116) |
| arm64, LLVM emitters | nothing | 0 functions | selfhost-arm64-2026-10-11.md, selfhost-llvm-2026-10-11.md |
| xo1 driver | `test --real`, contract fuzz listing, a test time limit | 3 features | selfhost-arm64-2026-10-11.md step 6 |

Next experiments, ranked by how directly they unblock the switch-over:

1. Run the full tier (`XO_SELFHOST_TIER=full go test ./internal/selfhost
   -run 'TestSelfhostChecker$'`) to confirm the arm64 and LLVM builds of
   the checker on the new code (escape.xo walks every scope body).
2. Let xo1 bind: when a `use go` or `use c` binding is missing, run
   `xo bind` (or `xo bind --c`) as a subprocess and read the cache,
   instead of reporting the run unsupported; then compare XO1101 and
   XO1104 bind failures.
3. The SSA builder's 15 runs both builders refuse: lower the constructs
   in internal/ssa first (the errors list them), then port.
