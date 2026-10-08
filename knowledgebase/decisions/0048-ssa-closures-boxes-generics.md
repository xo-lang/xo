# 0048: Closures, recursive types, and generics in the SSA IR

Status: Accepted (2026-10-08). Extends decisions 0045 (shared SSA IR)
and 0046 (reference counting). Implemented in `internal/ssa` (builder,
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

## Consequences
- The LLVM goldens with closures take the SSA path and free their
  environments (`rc_closures` builds and frees a chain of 100,000
  closures).
- `var` parameters of function values, methods as values, and generic
  functions as values are not supported (the builder reports them).

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
