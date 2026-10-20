# The Go backend failed to build tuples wider than six

Status: Observed and fixed up to 16 (2026-10-20)

Symptom: a program with a 12 element tuple (std/netip's differential
test, `Result[(Int, Str, Str, Str, Bool, ... Str), Str]`) checked and ran
natively, but `--backend=go` failed in `go build` with `undefined:
rt.Tuple12` ("a code generator bug").

Cause: the Go backend names a tuple type `rt.TupleN` for any N, and
std/rt defined Tuple2 to Tuple6 only.

Fix: std/rt/tuples_gen.go defines Tuple7 to Tuple16 with Tuple6's
methods (Eq, Hash, Ord, Debug, JSON, marking); golden tuple_wide covers
7, 12, and 16 element tuples on every backend. A tuple of more than 16
elements still fails this way on the Go backend; a checker limit (an XO
code) would need a compiler change, so prefer a struct there.
