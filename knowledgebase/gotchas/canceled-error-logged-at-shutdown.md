# A background loop logs `Canceled` as a failure at shutdown

Status: Observed (2026-10-08, notes-service, found by `xo test --explore=20`
with a `testing.logs()` assertion)

Symptom: under seed 2 the end to end test logged
`WARN purge failed err=Canceled` at the moment of SIGTERM. Seed 1 did
not, so the plain `xo test` run never showed it.

Cause: `purge_loop` matched `Err(e) => log.warn("purge failed", ...)`. When
shutdown cancels the background task while it is inside a database call
(rather than in `clock.sleep`, which unwinds), the call returns
`SqlErr.Canceled` (std.md 0: every blocking error type has `Canceled`),
and the catch all arm logged it as a failure.

Fix: match cancellation first and stop the loop:
`Err(Canceled) => return` before the catch all arm. Assert on
`testing.logs()` in shutdown tests and run them with `--explore` so the
interleaving where the cancel lands inside the call is tried.
