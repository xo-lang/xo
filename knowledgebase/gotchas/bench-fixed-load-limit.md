# xo bench --ab dropped most rounds at a fixed load limit

Status: Observed (2026-10-28)

Symptom: a quiet `xo bench --ab` (HEAVY_CLASS=measure) reports nearly
every round loaded and too few used, or prints no numbers: Lang 1's
runtime A/B lost 18 of 20 workloads (~/xo-lead/notes/lang1/rtab-loaded-1428.log),
and a path A/B started at load 6.65 dropped all 15 rounds.

Cause: with no --max-load, a round was loaded when either side started
at a 1 minute load of 6 or more. With seven sessions and screen sharing
the idle Mac sits at 4.4 to 5, and a multi-threaded workload's own
previous side pushes the next side over 6.

Fix: the limit is relative (compiler/cli/bench/bench.xo load_limit): the
load sampled before the first round plus LOAD_MARGIN (3), never under 6;
the summary's load line prints the baseline. --max-load still sets an
absolute limit and makes the guard wait.
