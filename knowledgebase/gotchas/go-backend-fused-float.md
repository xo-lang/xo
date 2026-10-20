# The Go backend fused Float multiply-adds on arm64

Status: Observed and fixed (2026-10-20, decision 0287)

Symptom: a loop updating `v = (v * 1.0071 + Float(i % 7) * 0.013).mod(40.0)`
gave a different value from iteration 13 on with `--backend=go` than on
the native backends (one unit in the last place), on an arm64 Mac.

Cause: the Go backend printed a Float product as `(a * b)`, and Go's
compiler may fuse `x*y + z` into one FMA instruction, rounding once
(the Go spec allows it "possibly across statements"; it does on arm64,
ppc64, s390x, and amd64 v3). Xo rounds every Float operation, as the
native backends do.

Fix: compiler/gen/expr.xo wraps a Float product in a conversion,
`float64(a * b)` (`float32` for Float32), which the Go spec says
forbids fusion. Golden float_no_fma checks a product whose fused and
rounded results differ, directly, through a variable, a function, a
loop, and Go's math, on every backend and LLVM level; it fails if a
backend starts fusing (an fmadd peephole, LLVM's `contract` flag).
