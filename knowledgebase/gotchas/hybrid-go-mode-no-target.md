# Go mode assembly cannot lower `target`

Status: Observed (2026-10-13, decision 0213)

Symptom: `go test -short ./internal/gen` failed TestHybridPreemption
with `std/native/llrt/sysio.xo:35:9: the SSA builder does not support
the name `target` here` after sysio.clock_now started choosing its OS
layer with `match target.os`.

Cause: a hybrid build (`--hybrid`, decision 0084) lowers some functions
to Go assembly with Options.NoFaultUnwind, where the builder does not
know the target (internal/ssa build_target.go returns nil). A spin loop
that calls Clock.now reaches the native prelude's llrt_clock_now and so
sysio.clock_now; the file functions are not reached there.

Fix: keep `target` out of the functions a hybrid build reaches
(Clock.now's path: sysio.clock_now calls rt.clock_now). When the clock
moves to Xo, the Go builder's clockNow can call the C runtime's
clock_now in Go mode (`|| s.b.opts.NoFaultUnwind`), which passed
TestHybridPreemption when tried.
