# xo bench --ab leaves out every round of a program that keeps all cores busy

Status: Observed (2026-11-22, task #687, host 1, 12 cores)

**Symptom.** A twin row for rwmutex (64 threads) or pmap (a process or
thread per core) prints its header and no table. Run by hand, `xo bench
--ab` says "no round was usable: every round ran under load".

**Cause.** Without `--max-load`, a round is loaded when it starts at or
above the baseline load plus 3 (compiler/cli/bench/bench.xo,
`load_limit`). A program that saturates the machine raises the 1 minute
load itself: from a baseline of about 2.9 the load passed 10 after the
first rounds, so every later round counted as loaded. The same program
measured from a baseline of 10.8 used every round. bench/perf/twins/lib.sh
also dropped xo bench's message (it kept only the table lines), so the
row was silently empty.

**Fix.** Give a saturating row an explicit `--max-load` (lib.sh: the CPU
count plus MAXLOAD for rwmutex and pmap, named in the row's header), and
print xo bench's output when it gives no table. bench/perf/twins/lib_test.sh
covers both.
