# Go backend: "xo internal: XoEq not generated for T"

Status: Observed (2026-11-07, decision 0542: `xs.contains(x)` and
`xs.index_of(x)` on a list of a user struct faulted on main c2f2c5617; the
prelude's element searches were not roots)

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

The other direction (Observed, 2026-11-07): a package keeps XoEq or
XoHash on many types nothing compares (9257e25d8 kept all 3,290 of
armnetwork's, since one generic form that ran set them for every type).
No output shows it. Build with `XO_GO_PROTO=why` to see each reason a
protocol is kept for every type, and pin the fix in internal/gen
TestGoProtoNotKept (a type the golden never compares keeps none).
