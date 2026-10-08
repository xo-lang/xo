# Jump tables are not always faster than compare chains on Apple M4

Status: Observed (2026-10-08, internal/arm64, bench interp and synthetic
`match` programs)

Symptom: a jump table made interp 1.8x faster, but the same table made a
10 arm `match` over a repeated list of random tags 3x slower than the
compare chain, and lost 7 to 30 percent on fully random tags at every
size from 4 to 16 arms. Separately, the old chain was slow on interp
only because of its layout: laying the tests out contiguously took
interp from 81 to 55 ms with the same instructions.

Cause: the indirect branch of a table resolves after a dependent load
(`ldrsw` from the table), so a mispredict costs more than a
conditional one, and the M4's conditional predictor learns long
repeating sequences the indirect predictor does not. A chain whose
failing tests are taken branches (reverse postorder interleaves the
arms) wastes fetch bandwidth and predictor history on every dispatch.

Fix: `internal/arm64/jumptab.go` lays out every chain as a run of tests
that fall through, and uses a table only from 8 distinct values
(decision 0051). To measure a threshold change, build with
`XO_ARM64_JT_MIN=n` (0 disables tables) and compare with
`bench/perf/llvm/native.sh` plus a random tag program; do not trust
one benchmark.
