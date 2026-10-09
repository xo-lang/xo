# Lowering deferred code overwrote the stack of open blocks

Status: Observed (2026-10-08, decision 0070)

Symptom: on the native backends a `defer` ran at the first exit of its
block but not at later ones. internal/llvm's signal program skipped its
shielded `defer { if mode == "hung" { ... } }` on every unwind path but
the first, so the third signal never found it hanging.

Cause: `exitTo` and `unwind` (internal/ssa/build_defer.go) lower each
open block's deferred code with `s.exits = saved[:i]`, the stack below
that block. The deferred code's own blocks (the `if`) push onto
`s.exits` with `append`, which reuses `saved`'s backing array and so
replaced `saved[i]` (the block whose defers were running) with the `if`
block's empty frame. Every later exit saw the wrong frame.

Fix: truncate with a full slice expression, `saved[:i:i]`, so an append
copies. The `exits` function of `internal/llvm/testdata/golden/defer_blocks`
lowers a deferred `if` at three exits. Any stack kept in a slice and
restored after nested lowering needs the same care.
