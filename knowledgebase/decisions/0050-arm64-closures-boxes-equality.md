# 0050: Closures, boxes, equality, and wide values in the arm64 backend

Status: Accepted (2026-10-08). Extends decision 0049 (native arm64
backend) with the SSA constructs of decision 0048. Implemented in
`internal/arm64` (`closures.go`, `boxes.go`, `eq.go`, `rc.go`,
`emit.go`).

## Context
After decision 0049 the arm64 backend ran 16 of the golden programs of
`internal/llvm`; the others used SSA ops and types it rejected: function
values (`closure`, `capture`, `callv`), boxes of recursive types (`box`,
`unbox`), `==` on Str, structs, tuples, enums, lists, and boxes, and
values wider than eight words in reference counting helpers and results.
The LLVM backend lowers all of them (`internal/llvm/fromssa.go`).

## Decision
- **Function values** are two integer words, code and environment. The
  environment is null without captures, else a cell
  `{rc, drop, capture words...}` from `xo_rc_alloc`, 8 bytes per word.
  `capture` loads words from it; `callv` passes the closure's words first
  (code in x0, environment in x1, then the arguments, AAPCS64) and calls
  `blr x0`, which keeps the code word in its ABI register instead of a
  scratch register. A drop that reaches one hands the cell to
  `xo_rc_defer` with the per function `envdrop` helper (releases the
  captures, frees the cell), so chains use the runtime worklist.
- **Boxes** are one integer word, a cell `{rc, T words...}`; `box` stores,
  `unbox` loads. A drop that reaches one goes through `xo_rc_defer` with
  a `boxdrop` helper. Layout no longer recurses through List or Box
  elements (both are pointers).
- **Equality**: a Str compares inline through `xo_str_eq`; structs,
  tuples, and enums call a generated `eq` helper with both values' words
  as integer arguments (floats as bits), which stores them in its frame
  and compares in memory; lists and boxes call a helper with the two
  pointers. Floats compare with IEEE equality (NaN unequal), enums by tag
  then the variant's fields, nested Str, List, and Box fields through
  their helpers. Equal box pointers short cut, list pointers do not (as
  LLVM). Ordering of aggregates stays an error (the checker never asks).
- **Wide values**: reference counting helpers take words past x7 on the
  stack like any call, copy them into their frame, and pass leaves past
  x7 the same way. Results past eight words of a class are stored by the
  callee in the caller's stack argument area (the caller reserves the
  slots as for stack arguments); the callee has read its own stack
  arguments at entry, so the area is free at return. This is the
  backend's own convention (Xo to Xo calls only), not AAPCS64's x8
  indirect result.

## Consequences
- All 35 programs of `internal/llvm/testdata/golden` run natively with
  their `want.txt` and no allocation live at exit; `TestGoldenRun` runs
  every one of them (no list). `TestUnsupported` builds a module by hand
  with `OpDrop`, the only op neither native backend lowers (InsertRC
  never emits it).
- Retaining a closure or a box calls its helper instead of an inline
  increment (a call per retain); equality of small structs calls a
  helper. Neither is on the bench programs' paths (timings unchanged,
  `bench/results/arm64-native-2026-10-08.md`).
- A recursive struct first met as its Option is not boxed by the SSA
  builder and is rejected by both native backends
  (`gotchas/ssa-option-first-recursive-type.md`).

## Alternatives
- **Indirect call through x16**: x16 is the parallel move cycle scratch
  register; x0 already holds the code word as parameter 0.
- **Equality by pointers to spilled values at the call site**: needs a
  frame area per call site in the allocator; passing words as arguments
  reuses the call machinery and stack arguments.
- **AAPCS64 indirect result (x8)**: needs a caller frame buffer and a
  pointer argument; the stack argument area already exists and is sized
  per call.
- **Inline equality for scalar only structs** (`cmp`/`ccmp` chains):
  faster, more code; worth it when a workload compares in a loop.
