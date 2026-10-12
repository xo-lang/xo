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

## Stages 2 and 3: parameters by reference, the memory of `var` parameters

Stage 2 (decisions 0145, 0147): struct and enum parameters of 256 bytes
or more of functions called directly arrive as pointers (`ptr noalias`,
`readonly` when not `var`), are loaded at entry, and are written back
at return; a call stores such arguments into the function's home slot
of their LLVM type or spare slots, passes read only memory on in place,
and reads the `var` results back. Stage 3 (decision 0148, held.xo): the
forward dataflow of what each `var` parameter's memory holds, field
reads and `setfield` in place, stores left out when the memory already
holds the value, and values never materialized. selfhost/llvm is about
3,700 lines with both (held.xo about 400).

All three stages measured on the same code (the final one, with
`--stage=N`; the emitter's own sources are a run of the corpus, so the
Go emitter's function count grew from 13,658 in the first stage 1 run
to 13,683 as selfhost/llvm grew):

| repository runs (318 compared), go build | identical | skipped | differing | modules byte identical |
|---|---|---|---|---|
| stage 1 | 10,232 of 13,683 | 3,451 | 0 | 285 of 318 |
| stage 2 | 11,362 of 13,683 | 2,321 | 0 | 309 of 318 |
| stage 3 | 13,683 of 13,683 | 0 | 0 | 318 of 318 |

| test binaries (46 compared), go build | identical | skipped | differing | modules byte identical |
|---|---|---|---|---|
| stage 1 | 4,491 of 5,497 | 1,006 | 0 | 32 of 46 |
| stage 2 | 4,948 of 5,497 | 549 | 0 | 44 of 46 |
| stage 3 | 5,497 of 5,497 | 0 | 0 | 46 of 46 |

16,016 helpers identical in the repository runs and 6,338 in the test
binaries at every stage. `TestSelfhostLLVMMutations` with stage 3 and
400 mutations (seed 20261011, go build): 282 lowered by both and
compared, 112 rejected by both, 6 not built completely by the Xo SSA
builder; 6,975 of 6,975 functions identical, 11,273 helpers identical,
282 of 282 modules byte identical. Every function stage 2 and stage 3 lowered was
identical on its first full run. With stage 3 every module of the 318
repository runs and the 46 test binaries both lower is byte identical
to internal/llvm's (the runs not compared are those the Go checker
rejects, those without `main`, `use go`, and the 15 runs the SSA
builders do not build completely, where the Go builder reports an
unsupported construct too).

Found in stages 2 and 3:

- **The other generated functions of the Xo SSA builder were in
  another order too** (stage 1's finding, continued): after
  `xo.debug[T]` was built when asked for, 17 modules were still not
  byte identical, every function identical and the definitions out of
  order (`zero.fn(...)`, the Debug text a frame's arguments ask for,
  and with them the numbering of string literals and the order of
  runtime declarations). The frame argument formatters
  (`xo.frameargs(...)`), the zero functions of function types
  (`zero.fn(...)`), slices, and list concatenation were queued for
  run_gens; the Go builder builds every one of them when asked for.
  selfhost/ssa build.xo `gen_now` built each at once (TestSelfhostSSA
  unchanged, 0 differences; the 17 modules byte identical). The arm64
  emitter's port (decision 0158) found and fixed the same on main at
  the same time (`build_gen`); the merge keeps main's version, and the
  figures after the merge are below.

Speed, ms for 10 lowerings (as the stage 1 table; stage 3, every
function lowered with every convention, so equal work), idle machine:

| input (functions) | Go emitter | Xo emitter, go build |
|---|---|---|
| selfhost/lexer (92) | 84 | 47 (0.56x) |
| selfhost/parser (377) | 925 | 833 (0.90x) |
| selfhost/check (1,027) | 16,725 | 28,062 (1.68x) |
| selfhost/ssa (1,655) | 45,924 | 131,512 (2.86x) |

The Go emitter itself takes 4.6 s to lower selfhost/ssa (not profiled
here; the decision 0148 dataflow keeps a copy of the state for every
instruction and iterates to a fixpoint, the first suspect). The Xo
port does the same copies and compares the states whole at each block;
the ratio grows with function size, which points at that dataflow too.
Profiling both is the first next experiment below.

## Stage 4: the self hosted pipeline end to end

`TestSelfhostLLVMEndToEnd` (internal/selfhost/llvm_e2e_test.go,
`XO_SELFHOST_E2E=ssa`, after merging main with the arm64 emitter): the
go build of selfhost/llvm (the Xo checker, the Xo SSA builder and its
passes, the Xo emitter in one binary) lowered selfhost/ssa in 26.7 s
to 398,565,495 bytes of IR (7,928,966 lines), **byte identical** to
internal/llvm's. clang built a binary from that IR (the LLVM backend's
build with `build.Options.LLVMIR`, binary cache off, -O2 -flto) in 33
minutes 58 seconds, and that binary's output on `--std=std --each
--stages selfhost/lexer` (1,528,585 bytes: the SSA module of the lexer
at every stage, the verifier's verdicts, and its mutants) is identical
to the go build of selfhost/ssa's. On selfhost/lexer (the full tier's
default) the same test takes 3 seconds: 708,883 bytes of IR, byte
identical, and the binary's token dump of two lexer sources identical
to the go build's (374,805 bytes).

## Speed after the receiver fix

The stage 3 table above was taken with by value receivers on the
emitter's read only helpers, which on the go backend made the next
element write of every per value list copy the whole list
(gotchas/selfhost-by-value-receiver-marks-lists.md). With `var`
receivers, after merging main, under heavy.sh's machine lock (no other
heavy run), ms for 10 lowerings:

| input (functions) | Go emitter | Xo emitter, go build |
|---|---|---|
| selfhost/lexer (92) | 95 | 49 (0.52x) |
| selfhost/parser (377) | 1,096 | 607 (0.55x) |
| selfhost/check (1,027) | 17,705 | 9,321 (0.53x) |
| selfhost/ssa (1,655) | 56,212 | 29,514 (0.53x) |

The go build of the Xo emitter takes about half the Go emitter's time
on every input, with the same output. Both are slow on large programs
(5.6 s for one lowering of selfhost/ssa by the Go emitter); not
profiled on the Go side here.

## Tests

- Merge tier of internal/selfhost after the merge with main
  (`go test ./internal/selfhost`, GOMAXPROCS=4, -p 1 -parallel 2):
  pass, 1,583 s for the package. TestSelfhostLLVM there (every third
  run): 110 runs compared, 8,570 of 8,570 functions identical, 110 of
  110 modules byte identical; test binaries 14 compared, 1,194 of
  1,194 functions, 14 of 14 modules; TestSelfhostLLVMMutations 30
  mutations, 19 compared, 483 of 483 functions, 19 of 19 modules.
  TestSelfhostLLVM took 74 s of it (including the go build of
  selfhost/llvm), more than the 15 s estimated from a warm run.
- No full tier run (owner's policy, 2026-10-11: the lead runs one per
  batch); the arm64 and LLVM builds of the Xo emitter have not been
  compared yet.

## Next experiments

1. **The arm64 and LLVM builds of the Xo emitter** in the full tier
   (`XO_SELFHOST_TIER=full go test ./internal/selfhost -run
   'TestSelfhostLLVM'`): the go build is the only one compared so far.
   Watch for the LLVM backend miscompile the arm64 port found
   (decision 0159).
2. **Profile the Go emitter on selfhost/ssa** (`go test -cpuprofile`
   around llvm.GenerateSSA, or `xo llvmdump --bench=3` under pprof):
   5.6 s per lowering; the decision 0148 dataflow keeps a copy of the
   state per instruction, the first suspect, and the same change would
   then be ported to held.xo.
3. **The self hosted pipeline as `xo build --backend=llvm`**: the
   selfhost/xo driver of decision 0158 with selfhost/llvm in place of
   selfhost/arm64, so the switch-over experiment covers both native
   backends.
4. **A merge tier near 15 s for TestSelfhostLLVM**: cache the go build
   of selfhost/llvm with selfhost/ssa's (they share every module but
   one), or lower more runs per process (batches of 40 now).
