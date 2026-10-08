# 0042: Two phase graceful shutdown

Status: Accepted (2026-10-08). Amends 0024 (signals) and core 7.5.

## Context
In stage 3 (`bench/results/stage3-2026-10-08.md`) the log pipeline had to
"stop reading, finish what is queued, flush, exit 0" on SIGTERM. Core 7.5
made the first signal a sticky cancellation, so the subject could only drain
inside shielded `defer` blocks, which worked but read as a workaround.

## Decision
The first signal sets a stop state (`task.stop_requested()`, `task.stopping()`
channel) and cancels nothing; servers stop accepting and finish in flight
requests. Cancellation of main's root scope follows after a grace period
(default 30s, `os.proc.set_grace(d)`) or a second signal; a third exits 130.
`os.proc.signal_after` in tests delivers successive signals.

## Consequences
Draining is ordinary code (`select` on `task.stopping()`). Programs that
ignore the stop state still shut down, after the grace period.
