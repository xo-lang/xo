# The Go backend has tuple types up to six elements only

Status: Observed (2026-10-12, decision 0182 stage 3)

Symptom: `xo check` and `xo build --backend=arm64` accepted a loop over
a list of 8-tuples in `selfhost/macho/elflink.xo`, but `xo test` (the Go
backend) failed to compile it: `macho/elflink.xo:1512:28: undefined:
rt.Tuple8`.

Cause: `std/rt/core.go` defines `Tuple2` to `Tuple6`; the Go code
generator names `rt.TupleN` for any arity, and the checker sets no
limit, so a wider tuple passes every step before `go build`.

Fix: keep tuples at six elements or fewer in code that the Go backend
builds (here a helper function with eight parameters replaced the
tuple list). Next step: add `Tuple7` and up to some bound to the Go
runtime, or have the checker report the arity the Go backend lacks.
