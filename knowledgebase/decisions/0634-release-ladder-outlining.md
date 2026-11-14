# 0634: Repeated release ladders become one local subroutine

Status: Proposed (Xo Lang 1, 2026-11-14). Native code generation only: no language or
output change.

## Context

The lead asked (2026-11-14) whether build_native (compiler/xo's largest function, 1,009,460
instructions, 5.4% of xo's 18.8 M on darwin/arm64) should be split. Its instructions are mostly
release ladders: runs of `ldr x0, [sp, #N]; bl <drop helper>` that release a scope's values on
its exits. Measured on xo's self build assembly (XO_KEEP_ASM, main b98479d92, a ladder being a
run of 8 or more such pairs; script ~/xo-lead/notes/lang1/ladders.py):

| | instructions | in ladders | exact repeats of an earlier ladder in the same function |
|---|---|---|---|
| xo, darwin/arm64 | 18.81 M | 1.85 M (9.8%) | 1.38 M (7.4%); 1.20 M in ladders of 32+ pairs |
| build_native | 1.01 M | 0.82 M | 0.70 M (813 ladders, 239 distinct) |
| gcp aiplatform/apiv1 test build, arm64 | 26.5 M | 36 K (0.1%) | 24 K (0.1%) |
| gcp aiplatform/apiv1 test build, amd64 | 21.1 M | 54 K (0.3%) | 36 K (0.2%) |

So the size is not the function's length but its copies: a scope with hundreds of owned locals
(build_native's match arms over every SSA op) releases the same slots, in the same order, on many
exits. Splitting build_native would move these copies into the pieces, not remove them. The repeats
in other functions (lower 15.8 K pairs, build_go 11.3 K, check_program_ov 10.7 K) are the same kind.

Decision 0623's shared release tails (SSA, reltail) and the tail merge of cold blocks
(compiler/arm64 tailmerge.xo) do not catch them: a repeated ladder sits in the middle of a block
(in build_native 440 of the repeats follow a call of an rc helper and 550 are followed by more
code), and its exits go on to different blocks, so no block tail matches.

## Decision (proposed)

1. A pass on a function's final arm64 instructions (after register allocation and the tail merge,
   so slots and helper names are fixed): it finds the ladders of at least LADDER_MIN pairs (16 to
   start, measured in step 1) that occur two or more times verbatim (the same slots, the same
   helpers, the same order), and the longest common suffixes among those that differ only in their
   first pairs.
2. The first copy of each becomes a local subroutine at the function's end: `str x30, [sp,
   #LR_SLOT]`, the ladder, `ldr x30, [sp, #LR_SLOT]`, `ret`; each copy, the first included, is one
   `bl` to it. LR_SLOT is one frame word added to a function that has a subroutine. A copy that
   shares only a suffix runs its own first pairs and then `bl`s the suffix's subroutine.
3. Only ladders whose calls cannot fault or unwind (the tail merge's tail_inst_ok rule: no landing
   pad, no fault side exit, no poll), so the landing pad and fault tables need no row for the
   subroutine's pcs. A ladder that holds anything else is left as it is.
4. x86 (amd64): the same pass, later and only if its measurement warrants it (point 4 of the
   cost). A `call` there pushes the return address, so the subroutine's slot offsets are each
   N + 8, and the unwind row for its pcs needs CFA = rsp + frame + 8 (`.cfi_adjust_cfa_offset
   8` around it, or the frame pointer if the function keeps one). Measured (above) amd64
   ladders on aiplatform are 0.2%; xo's own amd64 build is measured in step 0 before any x86 code.

Expected: xo's arm64 text about 7% smaller (1.2 to 1.38 M instructions, build_native about 0.69
M); programs without large scopes of owned values (aiplatform, the HTTP CRUD server) nearly
unchanged.

## Cost

- Run time: a `bl`, the save and restore of x30, and a `ret` (4 instructions) on every exit that
  takes an outlined ladder, against a ladder of at least 32 instructions (16 pairs): at most an
  eighth more instructions on that path, with fewer i-cache lines. The gate measures it: `xo bench
  --ab` of the HTTP CRUD server, the alloc bench, and xo's self build (wall and CPU) on linux/arm64
  host 1; LADDER_MIN goes up if any is slower.
- Unwinding and debugging: the subroutine lies inside its function's range, so the function's FDE
  covers it. The function saved x30 in its prologue (it calls drop helpers, so it is never a leaf),
  so the CFI rows for the subroutine's pcs stay those of the function body: CFA and the saved
  return address are unchanged, and the bl to the subroutine only overwrites the live x30, which no
  unwinder reads mid function. The fault trace, the profiler (frame pointer walk) and xo debug see a
  pc in the right function; its line is the first copy's (the line table maps the subroutine's pcs
  to the ladder they came from), not the exit the program took. xo debug's step over a scope exit
  lands in the subroutine: its stepping treats a bl to a pc inside the same function as a step into
  that pc, which the gate checks with the debugger tests.
- Compile time: one hash per ladder per function; a suffix search only among ladders in a function
  that has a repeat.

## Steps

0. Measure xo's own amd64 build (linux/amd64 host) with ladders.py; record it here.
1. compiler/arm64: the pass behind XO_PASSES=-ladders (default on after the gate), LADDER_MIN, a
   codegen pattern test (test/codegen) with a function whose scope of 20 owned Strs exits on three
   paths: one subroutine, three bls.
2. Gate as a codegen change: codegen patterns, the arm64 goldens (TestGoldenRun), fp and the self
   build, the debugger tests (cmd/xo TestDebug*) and the fault trace goldens (the arm64 goldens with a fault trace in want.txt), the --ab runs of
   Cost, xo's native __text before and after. The amd64 sample only if step 4 lands.
3. Record the measured __text (xo, aiplatform) and the --ab in Results.
4. x86, if step 0 shows more than 2% of xo's amd64 text in repeats.

## Consequences

- One frame word per function that has a subroutine.
- The line of an outlined ladder's pcs is its first copy's.
- Splitting build_native is not done: it would not remove the copies.

## Alternatives

- Split build_native (the lead's question): moves the copies, saves no instructions.
- Release in SSA through one shared exit block per scope (extend reltail to exits with different
  targets: a tail that ends by jumping back to a continuation needs an indirect jump or a
  continuation index, which the SSA lacks): the subroutine is that, on the machine level.
- A per-function release helper called with a slot bitmap: data dependent, slower per release.
- Machine outlining across functions (LLVM's MachineOutliner): the slots differ between functions,
  so the ladders do not match across them.
