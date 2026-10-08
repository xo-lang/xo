# The first SIGTERM no longer cancels (spec 0.7)

Status: Observed (2026-10-08, implementing decision 0042)

Symptom: a test or program written for spec 0.6 sends one SIGINT/SIGTERM
(or calls `os.proc.signal_after(d)` once) and expects main's root scope to
be canceled right away. Under 0.7 nothing is canceled for 30 seconds: the
first signal only requests a stop (core 7.5). A real process test then
waits the whole grace period; a virtual clock test passes but ends 30s of
virtual time later than it used to.

Fix: react to the stop (`select` on `task.stopping()`, poll
`task.stop_requested()`, or let `serve` return), send a second signal, or
call `os.proc.set_grace(0s)` for the old behavior. In tests,
`signal_after` called twice delivers two signals; the second cancels.

Related trap met while writing `internal/gen/testdata/signal`: `defer` is
block scoped (`defer-differs-from-go.md`), so `if mode == "hung" { defer
{...} }` runs the deferred block at the end of the `if`, not at the end of
the function. Put the condition inside the deferred block.
