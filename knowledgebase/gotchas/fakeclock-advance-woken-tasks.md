# Native FakeClock.advance returned before the tasks it woke ran

Status: Observed (2026-10-09, golden test_fakes run natively)

Symptom: a task sleeping 2s, then `advance(1s)` twice, then
`expect(hits.get()) == 1` failed natively: the woken task had not run.

Cause: `xo_test_advance` returned as soon as its own timer fired; the
sleeper's timer fired at the same instant and its task had not run yet.
std/rt's Advance lets the woken tasks run before returning.

Fix: after its timer, advance waits on a timer due now, which fires only
at the next move of the clock, when every context waits again
(`std/native/llrt/twrun.xo` advance; it was `std/llrt/xo_testing.c`).
