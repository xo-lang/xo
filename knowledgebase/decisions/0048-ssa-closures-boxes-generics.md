# 0048: Closures, recursive types, and generics in the SSA IR

Status: Accepted (2026-10-08). Extends decisions 0045 (shared SSA IR)
and 0046 (reference counting). The rule for which field of a recursive
type is boxed is superseded by 0051 (decided from the type definition
graph, not from the order types are first met). Implemented in `internal/ssa` (builder,
`rc.go`, verifier) and lowered by `internal/llvm/fromssa.go`, runtime in
`std/llrt/xo_rt.c`.

## Context
After decision 0046, 13 of the 29 LLVM goldens still fell back to the
AST path of internal/llvm, which never frees memory. Besides small gaps
(failing functions, `==` on aggregates, some List methods), three
constructs needed a representation in the SSA IR that the RC pass can
reason about: function values that capture bindings, types that contain
themselves by value, and generic functions.

## Decision

### Closures
- A function value has type `KFunc` (parameters and result). Its value
  is a code pointer and an environment; the environment is one heap
  leaf: a counted cell `{rc, drop, captures...}`, null when nothing is
  captured. Captures are by value at creation (core semantics), so the
  environment is immutable and never needs uniqueness.
- A function literal becomes its own `Func` with `Indirect` set: its
  parameter 0 is the closure itself (always borrowed), its other
  parameters are owned (a caller through a value cannot know the
  callee's borrow inference), and `Captures` gives the environment
  layout. `closure` builds the value (consumes the captures), `capture`
  reads one (a projection that borrows the environment), `callv` calls a
  value (borrows it, consumes the arguments).
- A named function used as a value gets a wrapper (`name.value`, also
  `Indirect`) that calls it directly, so the named function keeps its
  inferred borrows.
- Releasing an environment whose count reaches zero goes through
  `xo_rc_defer`: the drop function releases the captures and frees the
  cell, and objects that reach zero meanwhile are queued, not dropped
  recursively. A chain of closures capturing closures (or of boxes, see
  below) is freed with a worklist instead of the C stack.

### Recursive types
- A field of a user struct or enum whose type contains that struct or
  enum by value (directly or through structs, enums, and tuples) has
  type `Box[T]`: a pointer to an immutable counted cell `{rc, T}`. The
  builder decides while lowering the type: types are interned when
  first met, so the first type of any cycle to start construction sees
  the whole cycle when its field into it completes, and boxes exactly
  that field (`Node.next: Box[Node?]`, `Expr.Add(a: Box[Expr], ...)`).
  Prelude enums (Option, Result) are never boxed themselves.
- `box` copies a value into a new box (consumes it); `unbox` reads it (a
  projection that borrows the box). Every read of such a field or
  payload unboxes; every store boxes (`coerce`), so the rest of the
  builder never sees boxes. Boxes are immutable: an update rebuilds the
  enclosing value with a new box, so sharing a box between copies is
  value semantics, and no uniqueness check is needed.
- A box whose count reaches zero is dropped through `xo_rc_defer` (as
  closure environments): releasing a 1,000,000 node chain uses the
  worklist, not 1,000,000 nested calls.
- `==` compares boxes by contents (equal pointers short cut); it still
  recurses once per level of nesting.

### Generics
- Monomorphized in the SSA builder, before the IR exists: `declare`
  takes the concrete type arguments of a call (`check.Call.Subst`,
  resolved through the caller's own substitution) and creates one Func
  per function and list of type arguments (`largest[Int]`,
  `Stack.push[Str]`), whose body is built with that substitution
  applied to every checker type. This is the scheme of internal/gen's
  monomorphizer and of the AST path of internal/llvm (their code is
  backend specific, so the builder has its own `declare`). Closures in
  a generic body inherit the substitution. A generic function used as a
  value gets its type arguments by matching its signature against the
  function type the value has.
- So every instance has concrete types, and the RC pass and its
  borrow inference run per instance: `repeat[Int]` has no RC traffic,
  `repeat[Str]` retains each pushed string.

## Consequences
- The LLVM goldens with closures take the SSA path and free their
  environments (`rc_closures` builds and frees a chain of 100,000
  closures).
- `var` parameters of function values and methods as values are not
  supported (the builder reports them).
- The goldens with generic structs, methods, and functions take the SSA
  path (`misc`, `rc_generics`), and so do those with recursive types
  (`recursion`, `value_semantics`, `generics`, `rc_recursive`).
- Equality of deeply nested recursive values recurses on the C stack
  (one frame per level); an explicit stack for `xo.eq` is the fix if a
  program compares long chains.

## Alternatives
- **Closure conversion to a struct of captures passed by value**: no
  heap cell, but a function value's size would depend on what it
  captures, which a `KFunc` type (and a `List` of them) cannot know.
- **Capture by reference (shared cells)**: not Xo's semantics (captures
  see the value at creation).
- **Borrow inference through function values**: needs a whole program
  flow analysis of which functions reach which call sites; the wrapper
  keeps direct calls precise for the common case.
- **Recursive drop**: simplest, but a long chain recurses once per
  element; the main thread's 1 GiB stack hides it today, a task stack
  would not.
