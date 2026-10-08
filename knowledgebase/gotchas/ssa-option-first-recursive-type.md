# A recursive struct first met as `T?` is not boxed in the SSA IR

Status: Observed (2026-10-08, internal/ssa builder; affects the LLVM and
arm64 backends alike). Not fixed yet.

Symptom: a program whose first use of a recursive struct is its Option
fails to build natively:

```
struct Node { val: Int, next: Node? }
fn main(os: Os) {
  var head: Node? = None   // Node? met before Node
  ...
}
```

`--backend=llvm`: "the SSA lowering does not support the recursive type
`Node?`"; `--backend=arm64`: "recursive type `Node?` (not boxed)". The
Go backend builds it. Writing `let n = Node{...}` (or calling a
function that takes or returns a `Node`) before the first `Node?` makes
it build.

Cause: `builder.typ` interns types when first met and boxes a field
when the field's type contains the struct under construction by value
(`boxed`, `containsByValue`, decision 0048). When `Option[Node]` starts
first, it builds `Node` for its `Some` payload; `Node.next` then gets the
interned `Option[Node]`, whose variants are still empty, so
`containsByValue` says no; and prelude enums are never boxed themselves.
The cycle ends up with no box. Functions are built on demand from
`main`, so the order is the order of first use in the program, not of
declarations.

Fix (to do, in internal/ssa): when a type under construction is reached
while checking a field (variants or fields still empty), treat the
field as recursive if the path from that type to the struct went by
value only; or build the payload types of a prelude enum before
interning it. Regression program: the one above, with the first use of
`Node` after `head`. Until then, tests that need such a chain construct
the struct first (`internal/arm64/testdata/run/boxes`).
