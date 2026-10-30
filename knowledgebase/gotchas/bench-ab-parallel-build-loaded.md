# `xo bench --ab` of a parallel build discards every round as loaded

Status: Observed (2026-10-29, xo-lang-6)

Symptom: `xo bench --ab --rounds=5 A B -- build -o out compiler/xo` in a
quiet measurement window (load 3, nothing else running) reported "0 of
5 rounds used (5 loaded)" and no ratios: every round's load was 6.3 to
7.3 against the guard of baseline 2.96 + 3.

Cause: the load guard reads the 1 minute load average around each run,
and a self build runs 8 emit workers (XO_JOBS) plus the front end's
tasks for 45 s, which is enough to lift the average past the guard by
itself. The guard was made for small programs whose load comes from
other processes.

Fix: for a command that is itself parallel, pass `--keep-loaded` (the
rounds count; the loads are printed, so a real intruder still shows as
an outlier) or `--max-load=X` well above the build's own load. Still
run it in a measurement window: --keep-loaded only stops the guard
from discarding the build's own load.
