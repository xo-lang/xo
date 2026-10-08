# Harness validation: rescoring the Claude outputs (2026-10-08)

`bench/harness/score.py` (see `bench/harness/README.md`) was run on the
stored Claude outputs with the current toolchain (xo built from this
checkout, Go 1.26.1, the `.tools/bin` analyzers):

```
python3 bench/harness/score.py solutions --claude
python3 bench/harness/score.py planted --claude
```

The point is to check that the scorer measures what the published results
measured, before it is used on other models.

## Solutions (three arm table)

Every published number of `three-arm-2026-10-08.md` (tasks 01 to 08) and
`stage3-2026-10-08.md` (09, 10) is reproduced: runs to green per arm (from
the `BUILD_LOG.md` tables), Go and Xo lines, Go test functions, and Xo test
blocks, 10 of 10 tasks. Escaped strict findings on plain Go tasks 01 to 05
(3, 1, 3, 13, 1) match `go-baseline-2026-10-07.md` exactly.

| Measure | Plain Go | Strict Go | Xo |
|---|---|---|---|
| First try pass rate | 8/10 | 2/10 | 9/10 |
| Runs in total | 12 | 18 | 11 |
| Lines (code and tests) | 9630 | 10202 | 5211 |
| Green now (current toolchain) | 10/10 | 10/10 | 8/10 |
| Diagnostics on final code | 68 (escaped strict findings) | 0 | 2 |
| Hidden checks, task 01 (new) | 17/17 | 17/17 | 17/17 |

The two Xo cells that are not green today are explained, not scorer bugs:

- **03 retry client** fails `xo test`: contract fuzzing of `backoff` finds
  the integer overflow already reported in `gen-run-v0.4.md` (negative
  delays admitted by the invariant). Its 14 written tests pass. It also now
  gets an XO0305 warning (a `_` arm hides `InvalidUrl`, `Canceled`; spec
  0.6 rule, decision 0038).
- **06 job queue** fails `xo check` with XO0410 (`m[k]` on a `Map`,
  `queue.xo:179`), a spec 0.6 rule added after the subject wrote it on 0.4.

"Xo tests when first run" for the original cells comes from rerunning the
final code (their tests had never run when written), so 03 now reads
"fail" where the published table says "pass; contract fuzz finds a real
bug": the same fact. Hidden checks are new (17 black box HTTP checks of the
behavior task 01 states); all three arms pass them.

## Planted bugs

| Set | Published | Scored now |
|---|---|---|
| Xo, author 1 (`planted-xo`), X | 29/30 (12 inexpressible, 14 check, 3 test, 1 escaped) | 29/30 (12, 14, 3, 1) |
| Xo, author 1, N (control) | 16/20 | 16/20 |
| Xo, author 2 (`planted-xo2`), X | 23/30 caught, 25/30 caught or warned | 23/30 (6 inexpressible, 8 check, 9 test), 25/30 with the 2 warned escapes |
| Go plain, X | 9/30 | 9/30 |
| Go plain + strict, X | 13/30 | 14/30 raw, 13/30 after the same manual exclusion |
| Go, N (tests) | 17/20 | 17/20 |

Every one of the 50 Go variants gets the same plain result and the same set
of strict tools as `bench/planted/results.jsonl`. The raw 14 includes the
task 05 X3 staticcheck S1000 style warning that the published table
excluded by hand ("not a detection"); the scorer reports raw counts and the
report text names the exclusion. Every Xo variant of both authors matches
the published spec 0.6 numbers, so no Xo rule added after 0.6 changed a
planted result.

The first rescoring of the Go set gave 23/30 for plain + strict: nilaway
counted as a new finding on every variant of tasks 01, 03, and 04, because
the scorer built the base and the variants in differently named
directories and nilaway prints that name in its traces (gotcha
`nilaway-trace-names-work-dir.md`). Fixed before the numbers above.

## Cost of a scoring run

Solutions: about 12 minutes for 30 cells (the strict tools run twice for
plain Go: once as the loop, once for escaped findings). Planted: about 15
minutes for the 80 Xo variants and about 45 minutes for the 50 Go variants
(nilaway and `go test -race` per variant and base).
