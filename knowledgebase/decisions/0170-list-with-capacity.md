# 0170: `List[T].with_capacity(n)`

Status: Accepted (2026-10-11, assigned by the lead), implemented
(2026-10-11, every backend).

## Context

Code that knows how many elements a list will hold (a builder's value
and block lists, a result sized by its input) pays for growth: the Go
backend copies a full list at each doubling, and the profile of the go
build of the Xo SSA builder had memmove and list growth at about 10%
(bench/results/selfhost-ssa-2026-10-10.md, "The go build's
allocation"). The prelude had `with_capacity` for `Buf` and
`BytesBuilder` only; a `Buf` cannot be a struct field that keeps
growing as a `List` does, so it does not help `Module.new_func`.

## Decision

- `fn (static: List[T]) with_capacity(n: Int) -> List[T]` in the
  prelude: an empty list, equal to `[]` in every way the program can
  observe, whose first `n` pushes append without growing; `n` below 0
  counts as 0. Written `List[Int].with_capacity(n)`, or
  `List.with_capacity(n)` where the expected type gives `T`.
- Go backend: `rt.List_with_capacity` makes the backing array with that
  capacity and a block the list owns (`newBlock(0)`), so pushes take
  the unique owner's in place path.
- Native backends (arm64, LLVM): the SSA builder emits `list.new` with
  the size operand that `Map.keys`/`values`/`entries` already use
  (`xo_rl_new_n` in std/llrt, at least 4 elements); no new runtime
  entry. selfhost/ssa (call.xo) builds the same call, so TestSelfhostSSA
  compares it.
- The self hosted checker reads std/prelude.xo, so it knows the method
  without a change.

## Consequences

- One more static method on a built in type; capacity stays invisible
  (no `capacity()` accessor), so a backend may ignore the hint.
- Measured on selfhost/ssa's `Module.new_func` (the use that motivated
  it), giving each function's value, block, and edge lists room up front:
  no gain. With room for 256 values, 32 blocks, and 32 edges the builder
  ran 5% (selfhost/check) and 3.5% (selfhost/ssa) more instructions, with
  128, 16, 16 about 2% and 1% more; wall clock within noise
  (bench/results/selfhost-ssa-2026-10-10.md, "List.with_capacity in
  Module.new_func", 2026-10-12). Growth by doubling was not the cost the
  profile suggested; `new_func` is left as it was. The method stays for
  code whose sizes are known exactly (a result sized by its input).
- Golden program internal/llvm/testdata/golden/list_with_capacity (Go,
  LLVM, arm64): value semantics after a copy, growth past n, a negative
  n, struct and nested lists, no allocation live at exit.

## Alternatives

- `Buf[T]` in the builder's structs: a Buf is move only and freezes
  into a list; the builder updates its lists in place for its whole
  life.
- A `reserve(n)` method on a `var` list: also possible later; a
  constructor covers the known size case without a second way to
  mutate.
- Ignoring the size on native builds (as `Buf.with_capacity` does
  there): the runtime already had `xo_rl_new_n`, so using it costs
  nothing.
