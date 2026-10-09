# Go assembly has no callee saved registers, and morestack clobbers all

Status: Observed (2026-10-09, internal/arm64 Go mode, quicksort)

Symptom: the first Go mode quicksort kept `i`, the list's base and
length, and `hi` in stack slots for the whole function: every iteration
of the hot scan loops loaded and stored `i` through memory (a loop
carried store to load forwarding chain), although the only calls were
the two recursive ones after the loops.

Cause: Go's ABI0 (and ABIInternal) saves no register across a call, so
the allocator's call clobber blocked every register at each call and a
value live across one spilled for its whole life (the native target has
x19 to x28). Emulating callee saved registers between our own assembly
functions is not safe either: a callee's prologue may call
`runtime.morestack`, which runs Go code (stack growth, or a preemption
that schedules another goroutine) and returns with every general
register clobbered; only the arguments in memory survive.

Fix: in Go mode the call clobber is not an allocation constraint; after
allocation, every value in a register whose range covers a call is
stored to its own slot before the call and loaded after it
(`regalloc.go`, `inst.saves`, `goEmitCall`). The loops keep their
registers; each call pays one store and one load per live value.
Recursion still pays ABI0 (arguments in memory) and the stack check on
every call: recursive fib was 1.49x slower than the Go backend, so only
functions with a loop are selected.
