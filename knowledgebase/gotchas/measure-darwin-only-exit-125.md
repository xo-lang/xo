# suite=measure exits 125 before any measurement line

Status: Observed (2026-10-14, runs 38656053811 and 38656056910)

**Symptom.** `gh workflow run ci.yml -f suite=measure -f packages=text`
(or json) builds both programs (`linux/arm64: .../t-arm64`, `t-llvm`)
and then fails with "Process completed with exit code 125", printing no
measurement line. `measure testworld` works.

**Cause.** Most bench/perf measure.sh scripts were written for the Mac:
they count with `/usr/bin/time -l` and read "instructions retired". On
Linux `/usr/bin/time` is GNU time, which has no `-l`; it prints its usage
and exits 125, and `set -e` ends the script. perf and
kernel.perf_event_paranoid were not involved.

**Fix.** A measure.sh used on CI counts with
`perf stat -x, -e instructions:u` when `uname` is not Darwin (the
`count` function of bench/perf/testworld, text, json). `run_measure` in
scripts/ci/suite.sh now refuses a script without `perf stat` with a
clear message, checks that perf runs, and runs the branch's script
against main's sources when main's copy is still darwin-only.
