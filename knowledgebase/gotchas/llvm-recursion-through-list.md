# LLVM lowering rejected an enum holding a List of itself

Status: Observed (2026-10-09, decision 0083)

Symptom: `enum Node { ..., InList(x: Node, items: List[Node]) }` built
and ran with the arm64 backend but `--backend=llvm` failed with "the SSA
lowering does not support the recursive type `Node`", after the SSA
builder (and so the XO1201 check) had accepted it.

Cause: `internal/llvm`'s checkType, which guards against types that
contain themselves by value, walked into a List's element type, though a
list's elements live in its heap buffer (a List is a pointer).

Fix: checkType stops at lists, like boxes and function values. Golden
`rc_recursive_list` keeps the case; TestNoGCCheckAgrees (internal/build)
is the test that should catch the next disagreement between the builder
and a backend, so add a golden for any construct a backend lowers on its
own.
