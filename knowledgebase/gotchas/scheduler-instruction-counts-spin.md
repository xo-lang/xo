# Instruction counts of the task benchmarks follow the idle worker's spinning

Status: Observed (2026-10-13, decision 0212)

Symptom: the instructions retired by bench/perf/conc's pingpong with
XO_THREADS=1 moved by 10 to 30 percent between runs of one binary (the
minimum of 15 runs of the C path was 312M in one round and 378M in
another), so a 5 percent bar could not be read from whole runs; with
every CPU the spread was larger still.

Cause: XO_THREADS=1 still has two threads, the root thread that runs
`main` and one worker. Whichever waits spins (find_work's steal rounds,
XO_SPIN_ROUNDS) before it sleeps, and how many rounds it spins depends
on timing, not on the work: a slower or faster path elsewhere changes
the spin count. With the thieves' ring peek (rq_nonempty, in every
steal round) behind a call, pingpong on arm64 took 9.7 percent more
instructions per round trip than the C path; 4.7 percent once the peek
was inline again.

Fix: compare per operation: two builds with N and 3N operations
(spawn: 200 and 600 rounds of 1000 tasks; pingpong: 100k and 300k round
trips), the difference over 2N, medians of 9 runs
(bench/results/task-switch-2026-10-13.md). Spawn is then stable within
about 1 percent; pingpong still varies by about 15 percent, so its
result is a set of rounds, not one number. Keep spin loop work (the
thieves' peek) inline.
