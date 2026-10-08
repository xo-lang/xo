# 0045: Shared typed SSA IR for native backends

Status: Accepted (2026-10-08). Implemented in `internal/ssa`. Update
2026-10-08: the LLVM backend lowers from it, with reference counting
inserted by `InsertRC` (decision 0046); programs outside the SSA subset
fall back to the AST path.

## Context
Decision 0040 lists three native paths: the LLVM backend (prototype in
`internal/llvm`, which walks the typed AST directly), reference counting
with Perceus reuse (to stop the prototype leaking and to make list and
struct updates in place when unique), and possibly a direct arm64
backend. All three need the same things the AST does not give: explicit
control flow, explicit values with one definition each, and a place to
insert retain and release. Writing RC insertion against the AST walker in
`internal/llvm` would tie it to LLVM and repeat for every backend.

## Decision
Add `internal/ssa`, one typed SSA IR that every native backend lowers
from:

- **Shape.** Module, Func, Block, Value. Phis are block parameters (MLIR,
  Cranelift, Swift SIL style): a terminator's Edge carries the arguments.
  The entry block's parameters are the function parameters. Every block
  ends in exactly one terminator (`jump`, `br`, `ret`, `unreachable`,
  `fault`).
- **Types** are the backend's view: Int, Float, Bool, Str, unit, named
  struct and enum (Option and Result are ordinary enums), tuple, List,
  capability (Os, Stdio). Interned per module, compared by pointer.
  `Type.Heap()` marks what RC must track.
- **Value semantics are explicit.** No pointers, no loads or stores. A
  field update is `setfield` (a new value); list mutation is a call that
  returns the new list (`list.set`, `list.push`, `list.pop` returning
  `(List, Option)`). Mutable locals are SSA renamed (Braun et al. 2013);
  `var` parameters and receivers are returned as extra results and stored
  back by the caller. This is the form Perceus wants: once ownership is
  explicit, `list.set` on a uniquely owned list is in place.
- **Faults.** An Int operation, `ftoi`, `list.index`, or `list.set` with
  a `Pos` is checked and faults at that position; an empty `Pos` means
  proven safe (loop counters today; the range analysis of internal/gen
  can clear more later).
- **Runtime operations are calls** with dotted names (`list.len`,
  `str.concat`, `Stdio.println`, `Float.sqrt`), documented in
  `ssa.RuntimeCalls`, so the instruction set stays small; backends
  inline what they want.
- **RC hooks.** `retain`, `release`, `drop`, `reuse` are in the
  instruction set (verified to apply only to `Heap()` types) but the
  builder never emits them; the RC pass inserts them after building.
- **Verifier** (`Module.Verify`): terminators, edge arity and types,
  predecessor and successor agreement, dominance of every use (edge
  arguments are used at the end of the source block), and per op operand
  and result types, including calls of module functions.
- **Printer** (`Module.String`) is the golden test format.

## Consequences
- The LLVM backend keeps its AST walker for now (switching is a rewrite
  of about 4,000 lines). Next: an `internal/llvm` lowering from SSA for
  the subset, compared on the same goldens, then delete the AST path.
- The builder covers the subset the bench programs need (all six
  `bench/perf/llvm/xo` programs lower and verify, with goldens), not yet
  generics, closures, failing functions and `?`, `is`, list patterns,
  `xs.indexed()`, `first`, `last`, `or_fault`.
- Match is lowered as a chain of tag tests; a later pass can form a
  switch terminator.
- Dead unit values (`tuple` of `()`) are left in place; a DCE pass is
  cheap to add.

## Alternatives
- **Phi nodes in instruction lists** (LLVM style): equivalent, but block
  parameters make edge arguments explicit, which RC insertion needs (an
  edge argument transfers ownership), and avoid phi ordering rules.
- **Lower straight to LLVM with allocas and rely on mem2reg**: what the
  prototype does. It leaves no IR to run RC and reuse analysis on and
  does not serve a non LLVM backend.
- **Memory form (alloca, load, store) in our IR**: easier to lower to,
  but RC and reuse analysis on values need SSA anyway.
- **Reuse Go's `golang.org/x/tools/go/ssa`**: wrong source language and a
  non standard library dependency.
