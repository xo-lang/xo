# 0158: The arm64 emitter in Xo, in stages compared through `xo asmdump`, and the switch-over experiment

Status: Proposed (2026-10-11). Second stage of decision 0150's order
(after the SSA builder). Evidence:
`bench/results/selfhost-arm64-2026-10-11.md`.

## Context

Decision 0150 orders the self hosting stages after the checker: the SSA
lowering (done, `selfhost/ssa`, identical to internal/ssa on every
function of every run both build, after every pass), then the arm64
emitter (internal/arm64, about 11,000 lines of Go), then WebAssembly,
LLVM, and the Go code generator. Its switch-over point is a self hosted
compiler (lexer through arm64 emitter) that compiles itself to a fixed
point on arm64 hosts and passes the arm64 goldens.

The emitter is one pipeline per function (instruction selection with
its memos, value ranges, loop strength reduction, and the by reference
convention of decisions 0151 and 0153; the MIR passes: dead code,
threading, jump tables, compare selection, constant hoisting, flag
reuse, latch placement; register allocation; emission with frames,
fault stubs, call site tables, and the text peepholes) plus module
state shared between functions (the reference counting, equality, box,
and environment helpers, string and position literals, and the label
counters). A difference anywhere in a function's pipeline shows in its
text, so the comparison is the assembly text itself.

## Decision

**The dump.** `xo asmdump [--os=darwin|linux] [--no-inline] [--each]
[--test] [--bench=N] <dirs...>` (hidden) checks dirs as a native build
does, builds the SSA with `xo ssadump`'s options (fault positions
relative to the working directory, no release, the host target), runs
Verify, InsertRC, Verify, and prints what `arm64.Generate` makes for
the OS (default darwin), falling back to no inlining when the inlined
module does not verify, as `xo build --backend=arm64` does
(`build.Arm64AsmDump`). `--each` prints `=== asm <dir>` before each
run; a run the checker rejects prints `== rejected`. `--bench=N` builds
once and times the emitter alone (`arm64.Generate` on a fresh copy of
the module) N times.

The self hosted emitter (`selfhost/arm64`, `xo run selfhost/arm64 --
[--std=DIR] [--wd=DIR] [--os=OS] [--no-inline] [--each] [--test]
[--bench=N] <dirs...>`) checks with selfhost/check in native mode,
builds with selfhost/ssa and its passes (prune_unwind, add_landings,
cleanup, insert_rc), and prints the same text, then `== skipped` and
one line per function it did not emit (`<function>: <why>`). A run the
Xo SSA builder does not build completely prints `== unsupported <why>`.

**The comparison** (`TestSelfhostArm64`, internal/selfhost/
arm64_test.go): every repository run both SSA builders build
completely, each with its own process run (`--each`), split into
pieces:

- functions (`// func <name>` to the next piece), helpers (`// <kind>
  <type>`: dup, drop, share, dupp, dropp, eq, boxdrop, boxshare,
  envdrop, envshare), the entry (`xo_entry`), the call site tables of
  each function, and the literals (strings, positions);
- labels made from counters of the whole program are renamed before
  the comparison: `f<N>_` (the function's number) to `f#_`,
  `GCC_except_table_xo<N>` by function, helper labels to their kind
  and type, `xo_str<N>` and `xo_loc<N>` to their text; so a function
  the Xo emitter skips, and a function order that differs between the
  two SSA builders, shift no other piece;
- every function the Xo emitter emits must be identical to the Go
  emitter's of the same name (closures by rank, as decision 0150), and
  every helper and literal it emits must be one the Go emitter emits
  with the same text; skipped functions are counted, never compared;
- the whole text is also compared byte for byte, and the number of
  runs identical byte for byte is reported (the final stage requires
  every run without skipped functions and with the same function order
  to be byte identical).

Mutations (`TestSelfhostArm64Mutations`) apply decision 0150's code
mutations to the single directory programs and compare the same way.

**The stages** of the port, each committed with its counts in the
results file:

1. **Core**: the module driver, value layout, MIR, instruction
   selection of integer and Float arithmetic, comparisons, conversions,
   structs, tuples, enums, Option, constants, direct calls with the plain
   convention, runtime calls, lists and strings, retains and releases
   through the generated helpers, the memos and value ranges, loop
   strength reduction and rotation, every MIR pass, register allocation
   with spills and far frames, frames, fault stubs, emission, and the
   literals. Both sides run without inlining (`--no-inline`). A function
   with a construct of a later stage is skipped.
2. **Closures, boxes, equality, sharing**: closures and captures,
   calls through function values, function addresses, rebinds, boxes,
   the equality helpers, `share`, `reuse`, cancelled retain and release
   pairs.
3. **By reference parameters and landing pads**: decisions 0151 and
   0153 (byref.go: the homes, the dataflow, calls and returns by
   reference) and decision 0085's landing pads (call site tables, the
   personality routine).
4. **Inlining**: inline.go on the Xo SSA, the module verified again
   with selfhost/ssa's verifier and generated without inlining when it
   does not verify; both sides with their default options.

**The switch-over experiment** (after stage 4): `selfhost/xo`, a
compiler driver in Xo (check, SSA, passes, arm64 emitter, then `cc` to
assemble and link with the C runtime, as internal/build does), built by
the Go compiler with `--backend=arm64` (binary A); A compiles
`selfhost/xo` (binary B); B compiles it again (binary C). The fixed
point is the assembly B writes being byte identical to the assembly A
writes (and the binaries being identical when the linker is
deterministic); B also runs the arm64 golden programs
(internal/arm64/testdata/run) with the Go backend's output. The result
is recorded whatever it is. The released `xo` and what it runs do not
change: that is the owner's decision after the fixed point (decision
0150).

## Consequences

- A change to internal/arm64 must be mirrored in selfhost/arm64 in the
  same change once the comparison runs in the merge tier.
- Go mode (goasm.go, gosyntax.go: Go assembler for the Go backend's
  hybrid builds) and the layout instrument (align.go, `XO_ARM64_LAYOUT`)
  are not ported: the self hosted compiler builds native programs with
  the default layout. A layout other than the default is not compared.

## Alternatives

- **Compare whole files only**: one difference early in a program (a
  helper number, a literal label) would make every later function
  differ, and the two SSA builders order a few generated functions
  differently; the renaming keeps each difference local.
- **Compare the MIR before register allocation**: a dump of its own that
  the Go emitter does not have; the text after emission is what the
  assembler sees.
- **Port Go mode too**: it is for the Go backend's hybrid builds, which
  need the Go toolchain anyway.
