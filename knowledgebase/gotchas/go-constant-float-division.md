# `0.0 / 0.0` is a Go compile error

Status: Observed (2026-10-07, golden test `int_ops`)

Symptom: generated code `float64(0.0) / float64(0.0)` failed `go build`:
"division by zero". Xo floats are IEEE 754 at run time (NaN), but Go folds
constant expressions at compile time and rejects the division.

Fix: float `/` is emitted as `rt.FDiv(a, b)`, a call, so it is never a Go
constant expression. Integer operations already go through rt calls.
