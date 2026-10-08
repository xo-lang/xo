# A recursive struct first met as `T?` is not boxed in the SSA IR

Status: Observed (2026-10-08, internal/ssa builder; affected the LLVM
and arm64 backends alike). Resolved (2026-10-08, decision 0052).

Symptom: a program whose first use of a recursive struct is its Option
failed to build natively:

```
struct Node { val: Int, next: Node? }
fn main(os: Os) {
  var head: Node? = None   // Node? met before Node
  ...
}
```

`--backend=llvm`: "the SSA lowering does not support the recursive type
`Node?`"; `--backend=arm64`: "recursive type `Node?` (not boxed)". The
Go backend built it. Writing `let n = Node{...}` (or calling a
function that takes or returns a `Node`) before the first `Node?` made
it build.

Cause: `builder.typ` interned types when first met and boxed a field
when the field's SSA type contained the struct under construction by
value (decision 0048). When `Option[Node]` started first, it built
`Node` for its `Some` payload; `Node.next` then got the interned
`Option[Node]`, whose variants were still empty, so the check said no;
and prelude enums are never boxed themselves. The cycle ended up with
no box. Functions are built on demand from `main`, so the order was the
order of first use in the program, not of declarations.

Fix: `boxed` now decides from the checker's type definition graph
(`reachesByValue` in `internal/ssa/build.go`): a field of user type X is
boxed when its type reaches X by value through user types named after X
(prelude enums and tuples pass through), so each cycle is boxed at the
edge out of its least member, whatever order the program meets the
types in (decision 0052). Regression programs:
`internal/ssa/testdata/golden/recursive_order` and
`internal/llvm/testdata/golden/rc_recursive_order` (Option first, mutual
recursion, recursion through an enum payload, a 100,000 node chain
freed with 0 live allocations on LLVM and arm64).

Avoid: when adding a type rule to the SSA builder, do not decide it from
the SSA types interned so far; they are partial while a cycle is under
construction. Decide from the checker types.
