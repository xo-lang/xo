# `xo bench --ab` inside a gate deadlocks against a measure window

Status: Observed (2026-10-27, xo-lang-5's fa-tq gate and Lang 1's measurement)

Symptom: a gate (`HEAVY_CLASS=gate heavy.sh NAME SCRIPT`) whose script ran
`xo bench --ab` for a quick instruction count sat at 0% CPU for 28 minutes,
and a measure job queued behind it never started. The same script had run in
seconds earlier the same day, when no window was open.

Cause: `xo bench --ab` waits while the lead's quiet window is open (a future
epoch in ~/xo-lead/quiet) unless the run is the window's own (HEAVY_QUIET=1 or
XO_BENCH_IN_WINDOW=1). A measure job opens its window and then waits for the
running gates and slots to finish, so a gate whose --ab waits for that window
never finishes, and the window never closes. It looked like a heavy.sh inside
the script; it was the bench's own window wait.

Fix: since 2026-10-27 `xo bench --ab` exits 2 at once when heavy.sh started it
as a gate or a slot run (XO_HEAVY_SLOT set to anything but `measure`), is
not the window's own run, and would wait for the window (no --no-guard or
--guard=CMD), saying to run it as its own `HEAVY_CLASS=measure` job
(compiler/cli/bench/bench.xo slot_refusal; bench_test.xo). For an instruction
count inside a gate, read `/usr/bin/time -l`'s "instructions retired" line of
each binary instead: it waits for nothing.
