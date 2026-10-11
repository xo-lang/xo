# Self hosted LLVM emitter, 2026-10-11

Decision 0166. The LLVM emitter in Xo (`selfhost/llvm`) lowers the
module the self hosted SSA builder (`selfhost/ssa`, decision 0150) makes
after its passes and InsertRC, and must print the LLVM IR internal/llvm
prints for the same program (`xo llvmdump`, llvm.Generate without the
target's attributes), byte for byte. Apple M4 Pro, go build of the Xo
emitter unless a table says otherwise.

## How it is compared

`TestSelfhostLLVM` (internal/selfhost/llvm_test.go) runs both emitters
on every directory of the repository holding .xo files (the runs of
TestSelfhostSSA), each a run of its own, and on the test binaries of
the runs with test blocks; `TestSelfhostLLVMMutations` on mutations of
the single directory programs (TestSelfhostSSAMutations' edits, seed
20261011). The IR is split into its parts (llvm.SplitIR):

- every function the Xo emitter lowers (`define ... @"x.<name>"` and
  `@xo_entry`) must be identical to the Go emitter's definition of the
  same name, with string literals and fault locations compared by
  their contents (`@.rs.N`, `@.loc.N` are numbered in order of first
  use, which a skipped function shifts);
- every helper both emit (`xo.dup.T`, `xo.drop.T`, `xo.eq.T`,
  environments, boxes) must be identical; on a run with nothing
  skipped the helpers, named types, and runtime declarations must be
  the same sets;
- the whole module is compared byte for byte and counted (`modules byte
  identical`);
- functions the emitter leaves to a later stage are listed after `==
  skipped` and counted, never compared; runs the Xo SSA builder does
  not build completely (TestSelfhostSSA's skipped functions, which the
  Go builder does not build either) are counted, not lowered.

The figures below are the full corpus (`XO_SELFHOST_TIER=full
XO_SELFHOST_BACKEND=go go test ./internal/selfhost -run
'TestSelfhostLLVM$'`): 668 runs, of which 318 are lowered by both and
compared, 276 rejected by both checkers, 42 without `main`, 17 `use go`,
`use c`, or holes, and 15 the Xo SSA builder does not build completely;
the Go emitter reports an error for none.

## Stage 1: everything but parameters passed by reference

`selfhost/llvm` gen.xo (types, layout, globals, the reference counting,
equality, environment, and box helpers), func.xo (functions in reverse
postorder, phis, every instruction, landing pads, cancelled retain and
release pairs), call.xo (calls of module functions, the operations
lowered inline, the task runtime), main.xo (the command line), and
text.xo (the bytes of IR strings, a Float's bits), about 3,000 lines. A function with a parameter passed by reference (decision
0145: a struct or enum of 256 bytes or more of a function called
directly) and a function calling one are skipped.

| stage 1, go build | functions (Go emitter) | identical | skipped | differing | modules byte identical |
|---|---|---|---|---|---|
| repository runs (318 compared) | 13,658 | 10,227 (75%) | 3,431 | 0 | 273 of 318 |
| test binaries (46 compared) | 5,497 | 4,491 | 1,006 | 0 | 29 of 46 |
| 100 mutations (77 compared) | 1,735 | 1,667 | 68 | 0 | 67 of 77 |

16,004 helpers identical in the repository runs (6,338 in the test
binaries). Skipped: 3,266 functions with a parameter passed by
reference, 165 that call one. Every function lowered was identical on
the first full run but one: a function name holding `: `
(`xo.frameargs(req: T, ...)`) was cut at it when the test read the `==
skipped` lines (a harness fix, the last `: ` now separates the reason).

Found in stage 1:

- **The Xo SSA builder made the Debug text functions in another
  order.** It declared `xo.debug[T]` when asked for but built its body
  after the current one, so the functions that body declares (the Debug
  text of the parts, `xo.debug[Str]`) came later in the module than in
  the Go builder's, where genFunc builds the body at once. The SSA test
  compares functions by name, so it did not show; the LLVM module, which
  is written in the module's order (and numbers its string literals and
  defines its types in the order of first use), did: selfhost/lexer's
  module differed only by where `xo.debug[Str]` stood. selfhost/ssa
  show.xo now builds `xo.debug[T]` when asked for (decision 0150's
  rule for generated functions whose bodies declare functions);
  TestSelfhostSSA unchanged (0 differences).

Speed, ms for 10 lowerings of one module (the emitter alone: `xo
llvmdump --bench=10` times llvm.GenerateSSA, `selfhost/llvm --bench=10`
the Xo emitter, after one check, build, and InsertRC), idle machine:

| input (functions) | Go emitter | Xo emitter, stage 1, go build |
|---|---|---|
| selfhost/lexer (92) | 81 | 46 (0.57x) |
| selfhost/parser (377) | 900 | 439 (0.49x) |
| selfhost/check (1,027) | 15,954 | 7,181 (0.45x) |
| selfhost/ssa (1,654) | 43,393 | 19,494 (0.45x) |

Stage 1 skips the decision 0148 dataflow, which the Go emitter runs for
every function with a `var` parameter passed by reference, so these
ratios are not of equal work; the stage 3 table is.
