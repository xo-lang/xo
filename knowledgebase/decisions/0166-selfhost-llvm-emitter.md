# 0166: The LLVM emitter in Xo, in stages through `xo llvmdump`

Status: Proposed (2026-10-11), until every stage lands. Stages 1 to 3
implemented in `selfhost/llvm/` (gen.xo, func.xo, call.xo, text.xo,
held.xo, main.xo): every function of every module both emitters lower
is identical, and every such module byte identical; `cmd/xo/llvmdump.go` (hidden `xo llvmdump`),
`internal/llvm/split.go` (llvm.SplitIR) and llvm.PrepareSSA, and
`internal/selfhost/llvm_test.go` (`TestSelfhostLLVM`,
`TestSelfhostLLVMMutations`). Evidence:
`bench/results/selfhost-llvm-2026-10-11.md`. The second stage after the
SSA builder in decision 0150's amended order (2026-10-11: arm64 emitter,
then this, then the Go code generator, then the Go implementation is
retired); it does not depend on the arm64 emitter's port (decision
0158).

## Context

internal/llvm's GenerateSSA (fromssa.go and held.go, about 2,600 lines
of Go) lowers the SSA module after InsertRC to textual LLVM IR, which
clang compiles with the C runtime. The SSA builder and its passes are
self hosted (selfhost/ssa, decision 0150) and print the same module as
the Go builder at every stage. The emitter is the last piece the LLVM
backend needs before a self hosted compiler can produce LLVM builds.

Unlike the SSA dump, the IR has module wide state whose numbering
depends on the order of everything before it: string literals
(`@.rs.N`) and fault locations (`@.loc.N`) are numbered at their first
use, named types and helpers (`xo.dup.T`, `xo.eq.T`, environments) are
defined at their first use, runtime calls are declared at their first
use, and the functions follow the module's order. So the IR is
compared both function by function (with globals by contents) and as
a whole module.

## Decision

**The port** follows fromssa.go and held.go line by line, as the SSA
port followed internal/ssa: the same functions (sgen and sfn become one
`Gen` with the function's state reset per function), the same order of
every call that numbers or writes something (the operand texts are
computed in the Go code's argument order, since an operand read from
memory, decision 0148, emits a load and takes a temporary number), and
the same text. The IR's parts are lists of text joined at the end (a
Str concatenation would be quadratic on the go backend). Values,
blocks, edges, and types are the SSA tables' indexes; per value state
(aliases, addresses, lazy reads, cancelled releases) is a List indexed
by value. The block of a value is computed from the block lists (the
passes do not keep Value.block up to date). A Float constant's bits
are computed by scaling (the prelude has no Float64bits).

**The stages**, each compared on every repository run before the next:

1. **Core**: types and layout, globals, the reference counting,
   equality, environment, and box helpers, every instruction, calls of
   module functions, the operations lowered inline, the task runtime,
   landing pads (decision 0085), cancelled retain/release pairs, and
   the addresses of box contents and list elements under a borrowed
   root (decision 0147's first part). A function with a parameter
   passed by reference (decision 0145) and a function calling one are
   skipped.
2. **Parameters by reference** (decisions 0145, 0147): the convention
   of struct and enum parameters of 256 bytes or more, the home and
   spare slots of a call, the results read back, read only parameters
   passed on in place. A function with a `var` parameter by reference
   (a tracked memory, decision 0148) is skipped.
3. **The memory of `var` parameters** (decision 0148, held.go): the
   dataflow, field reads and writes in place, values never
   materialized.
4. **End to end**: the self hosted pipeline (selfhost/check,
   selfhost/ssa, selfhost/llvm) lowers selfhost/ssa (or a smaller self
   hosted package) to IR identical to the Go implementation's, and clang
   builds a working binary from it.

**The dump.** `xo llvmdump [--each] [--test] [--func=NAME] [--bench=N]
<dirs...>` (hidden) checks as a native build does and prints `== ir`
and the module llvm.Generate makes (no target attributes; fault
positions relative to the working directory), `== rejected`, or `==
error <message>`; `--bench=N` times llvm.GenerateSSA alone on one
module (llvm.PrepareSSA). The Xo emitter (`xo run selfhost/llvm --
[--std=DIR] [--wd=DIR] [--each] [--test] [--stage=N] [--bench=N]
<dirs...>`) prints the same, then `== skipped` and `<function>: <why>`
for the functions it leaves to a later stage; a run its SSA builder
does not build completely prints `== unsupported`.

**Comparison rules** (TestSelfhostLLVM, TestSelfhostLLVMMutations):

- every function the Xo emitter lowers is identical to the Go
  emitter's definition of the same name, with `@.rs.N` and `@.loc.N`
  replaced by their definitions on both sides;
- every helper both emit is identical; on a run with nothing skipped,
  the helpers, named types, and runtime declarations are the same sets;
- whole modules are compared byte for byte and counted; byte identical
  is the goal of every stage on the runs it skips nothing in;
- runs the Xo SSA builder does not build completely are counted, not
  lowered; a run the Go checker rejects must be rejected by the Xo
  checker; a Go emitter error must be an error of the Xo emitter.

Tiers (tier_test.go): -short every 25th run, merge every third run but
those of buildOnlyMerge and 30 mutations, full every run, every
backend, 400 mutations.

## Consequences

- A change to internal/llvm's lowering must be made in selfhost/llvm in
  the same change, or TestSelfhostLLVM fails (-short compares every
  25th run).
- The module's function order is part of what is compared: the Xo SSA
  builder now builds every generated function when asked for, as the Go
  builder does (`xo.debug[T]`, `xo.frameargs(...)`, `zero.fn(...)`,
  slices, list concatenation: selfhost/ssa `build_gen`, which decision
  0158 made at the same time), so the functions
  a body declares follow it; the SSA comparison by name had not
  required it.
- `ssa.faulting_rt` and `Builder.run_passes` are public in selfhost/ssa
  for the emitter.

## Alternatives

- **Feed the Go builder's SSA to the Xo emitter** (a text form parsed
  in Xo): isolates the emitter from the SSA port, but needs an SSA
  parser and does not test the pipeline the self hosted compiler runs;
  the SSA builder's output is already identical, so its module is used.
- **Compare only whole modules**: one function the emitter skips would
  make every later string literal number differ, and no stage could be
  measured before the last; function by function with globals by
  contents measures each stage, and the module count shows when the
  output is byte identical.
- **One stage**: the by reference conventions and the dataflow touch
  most functions of large programs; porting them after the core shows
  which differences belong to them.
