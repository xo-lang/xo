# Batched native `xo test`: one process per run against batched (2026-10-15)

Decision 0235. `xo test --no-gc` on the Mac (arm64 backend, xo built from
the change), through the lead's heavy.sh slot. Each row is one directory
in one mode: `process` is XO_TEST_BATCH=0 (one process per test and per
property case, the behavior before decision 0235), `batched` is the new
default. **procs** is the number of test processes the harness started
(counted with a `sh` wrapper on PATH). **cold** is an empty build
directory with the backend and binary caches off, one run. **warm** is
the median of five runs on the built directory. The script is
reproduced at the end; the build and check time is the same in both
modes, so the differences are the test processes.

| dir | result | mode | procs | cold s | warm s (median of 5) | warm runs s |
|---|---|---|---|---|---|---|
| selfhost/lexer | ok: 20 passed, 0 failed, 0 skipped | process | 614 | 4.575 | 4.653 |  4.691 4.537 4.570 4.653 4.655 |
| selfhost/lexer | ok: 20 passed, 0 failed, 0 skipped | batched | 1 | 1.250 | 1.244 |  1.547 1.246 1.238 1.244 1.230 |
| selfhost/parser | ok: 10 passed, 0 failed, 0 skipped | process | 10 | 2.210 | 1.581 |  2.305 1.581 1.536 1.587 1.575 |
| selfhost/parser | ok: 10 passed, 0 failed, 0 skipped | batched | 1 | 2.201 | 1.511 |  2.100 1.508 1.488 1.514 1.511 |
| selfhost/prune | ok: 11 passed, 0 failed, 0 skipped | process | 11 | 1.439 | 1.351 |  1.609 1.327 1.351 1.335 1.379 |
| selfhost/prune | ok: 11 passed, 0 failed, 0 skipped | batched | 1 | 1.345 | 1.296 |  1.527 1.305 1.289 1.296 1.267 |
| selfhost/macho | ok: 15 passed, 0 failed, 0 skipped | process | 15 | 1.787 | 1.534 |  1.934 1.534 1.528 1.498 1.558 |
| selfhost/macho | ok: 15 passed, 0 failed, 0 skipped | batched | 1 | 1.650 | 1.438 |  1.762 1.438 1.430 1.493 1.431 |
| examples/notes-service | ok: 16 passed, 0 failed, 0 skipped | process | 16 | 18.598 | 4.006 |  6.145 3.977 4.006 4.014 3.923 |
| examples/notes-service | ok: 16 passed, 0 failed, 0 skipped | batched | 1 | 18.423 | 3.846 |  5.720 3.830 3.846 3.853 3.813 |
| cmd/xo/testdata/nogc/same | FAIL: 13 passed, 5 failed, 0 skipped | process | 18 | 18.184 | 3.663 |  5.723 3.660 3.664 3.663 3.645 |
| cmd/xo/testdata/nogc/same | FAIL: 13 passed, 5 failed, 0 skipped | batched | 11 | 18.096 | 3.619 |  5.450 3.619 3.594 3.594 3.654 |
| cmd/xo/testdata/nogc/prop | FAIL: 3 passed, 17 failed, 0 skipped | process | 1222 | 8.051 | 8.008 |  8.227 7.957 7.953 8.008 8.031 |
| cmd/xo/testdata/nogc/prop | FAIL: 3 passed, 17 failed, 0 skipped | batched | 906 | 6.167 | 6.200 |  6.509 6.135 6.225 6.200 6.036 |
| bench/xo/v0.4/06-job-queue |   note: support: planned; build without --no-gc to use the Go backend until then | process |  | .733 | .724 |  .733 .720 .736 .720 .724 |
| bench/xo/v0.4/06-job-queue |   note: support: planned; build without --no-gc to use the Go backend until then | batched |  | .712 | .733 |  .733 .716 .738 .725 .738 |
| bench/xo/v0.6/10-log-pipeline | ok: 9 passed, 0 failed, 0 skipped | process | 9 | 1.625 | 1.406 |  1.798 1.427 1.406 1.402 1.383 |
| bench/xo/v0.6/10-log-pipeline | ok: 9 passed, 0 failed, 0 skipped | batched | 1 | 1.452 | 1.343 |  1.591 1.343 1.373 1.338 1.340 |

(bench/xo/v0.4/06-job-queue does not build natively yet: "support:
planned". Its rows are the time to report that.)

## Reading

- **Property tests are where the processes were.** selfhost/lexer has 6
  property tests of 100 cases: 614 processes, 4.65 s warm. Batched it
  starts 1 process and takes 1.24 s (3.7x faster). Each run costs about
  2 ms in the test binary, but the harness's `sh -c 'cd ... && exec env
  ... bin'` costs about 5 ms more per run.
- **Plain test blocks gain 4 to 7 percent** (60 to 160 ms on 9 to 16
  tests), because their warm time is mostly the build and check:
  notes-service 4.01 s to 3.85 s, macho 1.53 s to 1.44 s.
- **Failing fixtures keep their one-process costs.** cmd/xo
  testdata/nogc/same (5 failing tests) needs 11 processes batched: 1,
  plus 5 reruns alone, plus 5 fresh processes after them. nogc/prop's
  17 failing property tests are dominated by shrinking replays, which
  stay one process each (1222 to 906 processes, 8.0 s to 6.2 s).
- **Cold runs** are dominated by the build (notes-service 18.4 s), so
  batching saves the same absolute time there.
- What is left of a warm `xo test` is the build and check path: 1.2 to
  3.8 s here before any test runs (Xo Lang 3 is profiling it).

## Gate

Both modes over selfhost's xo unit tests (lexer, parser, check, prune,
macho, cli/servers, cli/cmdhelp), cmd/xo testdata/nogc (same, prop,
native, real with and without --real), examples/notes-service, the 28
internal/gen testing goldens (test_*), and bench/xo's test packages,
each with `-v` and `--json`. That is 107 comparisons per backend, on
arm64 and on LLVM. Output and exit status were compared byte for byte:
0 differ on either backend. TestNoGCTests and TestNoGCTestLeaks
(which now also runs every passing test of its packages in one batched
process under XO_LEAKCHECK=1 and requires 0 live) pass.

## Item 2: the test binary started directly (2026-10-15)

The harness started every run as `sh -c 'cd DIR && exec env ... BIN'`
(three execs). It now starts BIN directly with os.proc's cmd builder
(`dir`, `set_env`, PWD set as `cd` left it). Warm medians of 5 on arm64,
the xo of c2a32622 (old) against the change (new), each in its own
built directory; main had faster builds by then (decision 0236), so
these warm times are lower than the table above:

| dir | mode | old xo (sh -c) s | new xo (direct) s |
|---|---|---|---|
| selfhost/lexer | process | 4.121 | 1.960 |
| selfhost/lexer | batched | 0.954 | 0.941 |
| examples/notes-service | process | 2.336 | 2.262 |
| examples/notes-service | batched | 2.199 | 2.167 |
| cmd/xo/testdata/nogc/same | process | 2.091 | 2.010 |
| cmd/xo/testdata/nogc/same | batched | 2.008 | 1.973 |
| cmd/xo/testdata/nogc/prop | process | 7.393 | 2.845 |
| cmd/xo/testdata/nogc/prop | batched | 5.734 | 2.393 |

About 7 ms per run was the wrapper. Gate: the old and the new xo over
the same 107 test directories and flags as above, batched and with
XO_TEST_BATCH=0, on arm64 and LLVM: 428 comparisons, 0 differ.

How nogc/prop's shrink replays end (the next item's question; counted
with an `sh` wrapper on the old xo): 460 pass (exit 0), 58 fail (an
improvement, exit 1), 354 are not smaller than the best (exit 3), of
872 replays. A long lived binary survives only the passing ones unless
a not smaller case can skip its body without exiting.

## Item 1: shrinking over a pipe (2026-10-15)

Shrinking's replays go to one serving test binary (XO_TEST_SERVE): a
not smaller case skips its body in process, so only a failing replay
(an improvement) ends the binary. cmd/xo testdata/nogc/prop, warm runs
on arm64 (5 each, built directory):

| mode | warm runs s |
|---|---|
| XO_TEST_BATCH=0 (one process per run) | 3.244 2.118 2.523 4.716 2.073 |
| batched, shrinking served | 1.709 0.922 0.924 0.844 0.879 |

Median 2.52 s to 0.92 s (and 2.39 s with batching before this item).
Gate: batched against XO_TEST_BATCH=0 over the same 107 directories
and flags, arm64 and LLVM: 214 comparisons, 0 differ. A serving
binary whose every replay is not smaller (an empty best) runs 3
replays with 3 skips and `xo leakcheck: 0 live of 102 allocations`;
TestNoGCTestLeaks checks that now.

## Next

- Done: shrinking over a pipe (item 1 above).
- Done: the `sh -c ... env` wrapper per run (item 2 above).

## Script

```bash
#!/bin/bash
# measure.sh OUT: xo test --no-gc (arm64) wall clock, one process per run (XO_TEST_BATCH=0) against batched (1):
# cold = empty build dir and caches off, warm = built, median of 5; procs = processes the harness started
S=$SCRATCH
out=$1
cd $S/en
mkdir -p $S/tb/mb
now() { perl -MTime::HiRes=time -e 'printf "%.3f", time'; }
dirs="selfhost/lexer selfhost/parser selfhost/prune selfhost/macho examples/notes-service cmd/xo/testdata/nogc/same cmd/xo/testdata/nogc/prop bench/xo/v0.4/06-job-queue bench/xo/v0.6/10-log-pipeline"
echo "| dir | result | mode | procs | cold s | warm s (median of 5) | warm runs s |" > $out
echo "|---|---|---|---|---|---|---|" >> $out
for d in $dirs; do
  for m in 0 1; do
    b=$S/tb/mb/$(echo $d | tr / _)-$m
    rm -rf $b; mkdir -p $b
    t0=$(now); XO_BACKEND_CACHE=off XO_BIN_CACHE=off XO_TEST_BATCH=$m XO_BUILD_DIR=$b/build bin/xo test --no-gc $d > $b/out 2>&1; t1=$(now)
    cold=$(echo "$t1 - $t0" | bc)
    ws=""
    for k in 1 2 3 4 5; do
      t0=$(now); XO_TEST_BATCH=$m XO_BUILD_DIR=$b/build bin/xo test --no-gc $d > $b/out 2>&1; t1=$(now)
      ws="$ws $(echo "$t1 - $t0" | bc)"
    done
    rm -f $S/tb/shw.log
    PATH=$S/tb/shw:$PATH XO_TEST_BATCH=$m XO_BUILD_DIR=$b/build bin/xo test --no-gc $d > /dev/null 2>&1
    procs=$(wc -l < $S/tb/shw.log | tr -d ' ')
    med=$(echo $ws | tr ' ' '\n' | sort -n | sed -n 3p)
    echo "| $d | $(tail -1 $b/out) | $([ $m = 0 ] && echo process || echo batched) | $procs | $cold | $med | $ws |" >> $out
  done
done
cat $out
```

The `sh` wrapper on PATH appends one line per call to $SCRATCH/tb/shw.log and execs /bin/sh.
