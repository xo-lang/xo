# 0051: Box recursive types from the type definition graph

Status: Accepted (2026-10-08). Supersedes the "builder decides while
lowering the type" rule of decision 0048 (Recursive types); the rest of
0048 stands. Implemented in `internal/ssa/build.go` (`boxed`,
`reachesByValue`).

## Context
Decision 0048 boxed the field through which the first type of a cycle
to start construction saw the whole cycle, using the SSA types interned
so far. When a program met `Node?` before `Node`, `Option[Node]` was
interned with empty variants while `Node.next` was checked, so no field
of the cycle was boxed and both native backends rejected the program
(`gotchas/ssa-option-first-recursive-type.md`). Which field got the box
also depended on the order functions were lowered.

## Decision
- Boxing is decided on the checker's type definition graph (struct
  fields, enum payloads including prelude Option and Result, tuple
  elements, aliases; type arguments substituted), not on the SSA types
  built so far. Lists, function values, and builtins are not by-value
  edges.
- Each by-value cycle is boxed once, at the edge out of its least user
  type by name (the instantiated name, `Tree[Int]`): a field of user type
  X is boxed when its type reaches X through user types whose names sort
  after X. Prelude enums and tuples are passed through and never boxed
  themselves. Every cycle has exactly that edge boxed for its least
  member, so every cycle is broken, and the choice is a property of the
  types alone.
- A self recursive type (`Node.next: Box[Node?]`, `Expr.Add(a:
  Box[Expr], ...)`) is boxed as before.

## Consequences
- Programs build the same SSA types whatever order they use the types
  in; `Node?` first now builds on the LLVM and arm64 backends.
- For mutual recursion the boxed field may differ from 0048's choice
  (`Dept.head` rather than whichever type was met first); the number of
  boxes per simple cycle is still one.
- A field on several cycles is boxed if it is the least member's edge of
  any of them; overlapping cycles may get more than one box, never zero.
- Regression programs: `internal/ssa/testdata/golden/recursive_order`
  and `internal/llvm/testdata/golden/rc_recursive_order` (Option first,
  mutual recursion, recursion through a user enum payload and a tuple,
  a 100,000 node chain freed with 0 live allocations on LLVM and arm64).

## Alternatives
- Build a prelude enum's payload types before interning it: fixes the
  `T?` case only; the result still depends on encounter order.
- Box every field on a cycle back to its own type: simpler, but a cycle
  through n user types gets n boxes and n allocations per node.
- Box the DFS back edges from a root chosen per strongly connected
  component: also order independent, but needs an SCC pass for one
  bit per field; the least name rule gives the same guarantee locally.
