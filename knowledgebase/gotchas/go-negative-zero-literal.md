# `-0.0` prints `0.0` on the Go backend

Status: Observed (2026-10-08, LLVM golden test `arith`)

Symptom: `os.stdio.println("${-0.0}")` prints `0.0` with the Go backend
and `-0.0` with the LLVM backend. `let z = 0.0` then `${-z}` prints
`-0.0` on both.

Cause: the Go backend emits `(-float64(0.0))`, a Go constant expression,
and Go constants are exact numbers without a negative zero; the value is
+0 at run time. LLVM's `fneg` of 0.0 is -0.0 (IEEE 754), which is what
the spec's "64 bit IEEE 754" says.

Fix (not done): the Go backend should negate float literals at run time
(as it divides floats through `rt.FDiv`, `go-constant-float-division.md`)
or emit `math.Copysign(0, -1)` for a negative zero literal. The LLVM
golden tests avoid the case.
