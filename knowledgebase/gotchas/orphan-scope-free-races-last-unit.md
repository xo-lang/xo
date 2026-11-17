# A scope freed by its owner's free races its last task's end

Status: Observed (2026-11-16, task #206)

## Symptom

`xo test --race` on a program that opens an in-memory server's stream
and drops it unread (xw-1's std/sse run on linux/arm64) reports a
ThreadSanitizer data race: `heapos.release` (from `srv_free`) frees a
task scope after `sched.scope_units_done`'s atomic write on another
thread, with no happens-before between them. Without the race detector
the same window is a use after free that rarely shows.

## Cause

A server that never reached its close freed its scope in `srv_free`
(`tasks.h_release`), on the reasoning that its tasks hold the server, so
none is left. But a task drops its reference to the server before its
run's last touch of the scope (`run_claimed` calls
`scope_units_done` after the task body returns), so the free can land
between the two.

## Fix

`tasks.scope_orphan` marks such a scope (`S_JOINER = SCOPE_ORPHAN`):
with no unit pending it frees the scope at once; otherwise it sets the
waiting bit and the last unit, instead of waking a joiner, frees it
(`pool.run_item`, `run_claimed`), with `sanhook.release`/`acquire`
pairing the two threads for TSan. Test: internal/llvm
TestRaceOrphanScope (`testdata/race_orphan`, 3 rounds).

When an object owns a scope whose tasks may outlive the object's last
reference, never free the scope directly: hand it to the scope's own
unit count.
