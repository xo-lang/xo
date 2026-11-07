# Go backend: "xo internal: XoEq not generated for T"

Status: Anticipated (2026-11-07, decision 0542)

Symptom: a program built with `--backend=go` faults at run time with
`xo internal: XoEq not generated for main.T (decision 0542; XO_GO_PROTO=all
writes every one)` (or XoHash, or `XoGenAny not generated` from a property
test), while the native and LLVM builds of the same program run.

Cause: since decision 0542 the Go backend writes a user type's XoEq, XoEqT,
XoHash and XoGenAny only when a whole-program walk reaches the type from a
root (`==`, a literal pattern, a Map key or Set element, a constrained type
argument, a type converted to an interface, `testing.gen`, a property
test's parameters, a derive of Eq, Hash or Ord). A way of comparing,
hashing or generating a value that the walk does not know is not a root,
so the method is missing and the runtime's EqAny, HashAny or Gen faults
instead of using reflection (which gives wrong answers for Xo values).

Fix: as a stopgap, build with `XO_GO_PROTO=all`, which writes every
protocol method as before 0542. Then add the missing root to
compiler/gen/proto.xo (proto_root at the generator site, or a case in
proto_close or proto_walk) and a case to the golden
internal/llvm/testdata/golden/go_proto_roots that fails without it.
