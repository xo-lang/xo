# Recording a call site on every block doubled channel handoff cost

Status: Observed (2026-10-08, deadlock reports and `xo dump`)

Symptom: after blocked tasks started recording where they wait, 300k
unbuffered channel handoffs went from 0.23s to 0.47s; `ChanPingPong` in
std/rt went from 274 to 755 ns per handoff.

Cause: `runtime.Callers` walks the stack: about 120 to 370 ns depending
on depth, even for a single frame (184 ns at depth 6). Both sides of an
unbuffered handoff block, so each handoff paid it twice.

Fix (std/rt/sched.go `setWait`): only test worlds and the deterministic
scheduler record the call site eagerly. A plain real run records the
wait kind and object only; the first time a thread blocks it labels its
goroutine `xo_thread=<id>` with `runtime/pprof`, and a dump or deadlock
report reads the call sites of all blocked threads from the goroutine
profile (`lazyLocs`). Spawned and HTTP request goroutines clear the
inherited label first. Cost is back to parity (0.21s).
