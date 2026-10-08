# A fault raised as a plain panic value must be converted while in flight

Status: Observed (2026-10-08, typed Int helpers in std/rt/mono.go)

Symptom: to fit Go's inlining budget, typed Int helpers fault with
`panic(&arithFault{...})` instead of calling `Faultf`, which builds a
`*FaultErr` with the stack at the call. Converted late, the fault's stack
would list the wrong frames: the frames of a re-panic site instead of the
operation, and a generated `defer` closure (`F_main.func1`) that is not on
the operation's stack.

Cause: Go runs deferred calls on top of the panicking stack, so a
`runtime.Callers` in a deferred function still sees the operation's frames
below `runtime.gopanic`. But a recover site that re-panics the raw value
(`Scope.Close` re-panics after waiting for tasks) starts a second panic, and
the innermost `runtime.gopanic` is then the re-panic.

Fix: convert at the first recover. `AsFault` handles `*arithFault`
(`arithFault.fault` takes the stack with `panicStack`: the frames below the
innermost `runtime.gopanic`, at most 62, as many as `stack(3)` keeps for a
fault raised in place), and `Scope.Close` converts before it re-panics.
Every other recover site in rt calls `AsFault` before re-panicking
anything but unwinds. A new recover site must do the same. Checked by
comparing fault output (messages and Go frames, including the 62 frame
truncation) of the old and new compilers on nested, scoped, task, and deep
recursion faults.
