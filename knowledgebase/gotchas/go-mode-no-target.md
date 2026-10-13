# Go mode SSA has no `target`: a runtime routine that tests it breaks Go backend assembly

Status: Observed (2026-10-13, decision 0206 stage 2).

Symptom: `TestHybridPreemption` (internal/gen, -short) failed with

```
std/native/llrt/sysio.xo:29:9: the SSA builder does not support the name `target` here
```

once Clock.now went through sysio.clock_now's Linux test on every
target.

Cause: the Go backend's assembly mode (internal/arm64 Go mode, built
with `NoFaultUnwind`) lowers no `target` (internal/ssa build_target.go
returns nil), and Clock.now became a call of a native prelude function
whose body matches on `target.os`.

Fix: in Go mode the SSA builder keeps Clock.now on the C runtime's
`clock_now` (internal/ssa build_io.go clockNow). Any other native
prelude routine that tests `target` and is reachable from code the Go
backend assembles needs the same guard.
