# `-0.0` is positive zero in generated Go

Status: Observed (2026-10-08, golden test `float_order`). Fixed the same day.

Symptom: `1.0 / -0.0` printed `+Inf` with the Go backend and `-Inf` with
the LLVM and arm64 backends; a list literal `[-0.0, 0.0]` sorted and
printed as `0.0 0.0`.

Cause: unary minus on a Float was emitted as `(-float64(0.0))`, a Go
constant expression. Go constants are exact and have no negative zero,
so the result is `+0`. The same trap as `go-constant-float-division.md`.

Fix: Float negation is emitted as `rt.FNeg(x)` (`std/rt/main.go`), a
call, so it is never a constant expression and the hardware negation
flips the sign bit. Spec: core 2.1 says `-0.0` is negative zero
(decisions/0053).
