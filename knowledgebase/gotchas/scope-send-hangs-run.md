# A blocked send inside a scope hangs `xo run` instead of failing

Status: Observed (2026-10-08, writing the map_value_semantics golden)

Symptom: a program whose spawned task sends on an unbuffered channel that
is only received after the `scope` block never exits under `xo run` (or a
built binary); nothing is printed after the last line before the scope
ends. `xo test` would report the deadlock (its virtual clock sees every
task blocked), but `xo run` uses real capabilities and has no such check,
and Go's own "all goroutines are asleep" detector does not fire because
the runtime keeps other goroutines (signal handling) alive.

Cause: the scope waits for its tasks; the task waits for a receiver that
runs after the scope. A plain deadlock in the program, not a runtime bug.

Fix: give the channel a buffer (`Chan[T].new(cap: 1)`) or receive inside
the scope. When a golden program hangs, list its processes and stop only
the binary that this session started (`kill <PID>`), never by name.
