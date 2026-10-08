# Tuples of more than six elements fail the Go build

Status: Observed (2026-10-08, while writing the LLVM golden test `misc`)

Symptom: a program with an 11 element tuple passed `xo check` but
`xo build` (Go backend) failed with "go build for darwin/arm64 failed
(code generator bug): undefined: rt.Tuple11".

Cause: the Go backend lowers an n element tuple to `rt.TupleN`, and
`std/rt/core.go` declares only `Tuple2` to `Tuple6`. The checker has no
arity limit. (The LLVM backend lowers tuples to literal structs of any
size.)

Fix (not done): generate the missing `TupleN` types per program, or
declare them up to a limit and make the checker report larger tuples.
Workaround: use a struct.
