# The Go backend fuses `a * b + c` on arm64; native code does not

Status: Observed (2026-10-09, `xo build --hybrid`, TestHybridRandom)

Symptom: a random differential program printed `dot 10471499` with the
kernel in Go assembler and `dot 10471500` with the Go version
(`Int(s * 1000.0)` of `s = s + Float(p.a) * p.b` summed over a List).

Cause: the Go spec lets an implementation fuse a multiply and an add
into one rounding (FMA), across statements too, and Go's arm64 compiler
does (`FMADDD`/`FMSUBD` when the product has one use). The arm64 and
LLVM backends round the product and the sum separately, so Float results
of the Go backend can differ from native code (and from hybrid assembly)
in the last bit. Xo's spec does not say either way.

Fix: none needed for correctness; differential tests must not depend on
it. TestHybridRandom's kernels never feed a Float product into an
addition (they divide instead). If Xo ever promises bit identical Float
results across backends, the Go backend must write `float64(a*b) + c`
(an explicit conversion stops fusion) or the native backends must emit
`fmadd` where Go would.
