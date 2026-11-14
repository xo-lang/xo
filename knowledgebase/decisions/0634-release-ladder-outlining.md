# 0634: Repeated release ladders become one local subroutine

Status: Accepted (lead, 2026-11-14: compiler internal, with an off switch; on the condition that
the fault trace and the profiler name the line of the copy that ran). Built by Xo Lang 1,
2026-11-14. Native code generation only: no language or output change.

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

The asm counts above overstate the program (gotchas/kept-asm-counts-overstate-text.md): xo's
linked binary has 9.32 M instructions, 0.53 M in ladders of 4 or more pairs and 0.50 M (5.3%) in
exact repeats (otool -tV, ~/xo-lead/notes/lang1/ladders-bin.py).

## Decision

1. A text pass on each function's final arm64 lines (compiler/arm64 ladders.xo outline_ladders,
   after far_slots_l and far_tests_l, before local_empty_l; XO_PASSES=-ladders turns it off) finds
   the runs of at least LADDER_MIN (16) pairs `ldr x0, [sp, #N]; bl SYM` that the function repeats
   verbatim (the same slots, the same callees, the same order). A run may hold line table rows
   (`_dl`), variable range boundaries (`_dv`) and cause marks (`_dc`), which no branch reaches; a
   block's label, a call site's (a landing pad) or a preemption span's ends it, so no outlined call
   has a landing pad. A slot past N = 32744 (whose N + 16 leaves the scaled ldr's reach) ends it;
   a far frame's far slots go through x28 before the pass and never match.
2. Each repeated run is written once past the function's body as a local subroutine with a frame
   record of its own: `stp x29, x30, [sp, #-16]!; mov x29, sp`, the run with each slot read 16 bytes
   further, `ldp x29, x30, [sp], #16; ret`, in an unwind region of its own (`.cfi_endproc` and
   `.cfi_startproc` before the function's last `.cfi_endproc`, as a leaf's fault stubs, with exact
   CFI rows for each instruction of its prologue and epilogue). Each copy becomes its labels and one
   `bl` to it. Exact runs only: sharing common suffixes as well (a trie, as tail_merge_cold) was
   estimated at about the same saving on xo (454 K instructions against the exact runs' 456 K), so
   it is not done.
3. The fault trace (llrt prof.xo trace_text_at), CPU profiles (cpu_section) and compiler/debug's
   frames (vframes: xo debug and `xo dev symbolize`) drop a frame whose pc has no line when the next
   frame is the same function at a line: the subroutine's, whose caller frame (from its frame
   record) is the function at the `bl` of the copy that ran, at that copy's line. A recursion's
   frames each have a line, and a build without DWARF has none for either, so neither folds. Heap
   profiles are not folded: a ladder releases, it never allocates.
4. x86 (amd64) and windows/arm64 keep their ladders: a `call` moves rsp (the slots would be N + 8
   and the CFA rsp + 8), and windows/arm64's unwind codes describe one prologue per region. x86
   follows only if xo's own amd64 binary shows more than 2% of its text in repeats (step 0).

## Cost

- Run time: a `bl`, the save and restore of x30, and a `ret` (4 instructions) on every exit that
  takes an outlined ladder, against a ladder of at least 32 instructions (16 pairs): at most an
  eighth more instructions on that path, with fewer i-cache lines. The gate measures it: `xo bench
  --ab` of the HTTP CRUD server, the alloc bench, and xo's self build (wall and CPU) on linux/arm64
  host 1; LADDER_MIN goes up if any is slower.
- Run time also: 16 bytes of stack below the function's frame while a subroutine runs, as any
  callee's frame record.
- Unwinding and debugging: the subroutine's frame record and CFI make it an ordinary frame for the
  frame pointer walk and for a CFI unwinder; point 3 folds it away where frames are shown.
  Checked: `xo dev symbolize` of a report whose first pc is in the subroutine prints only the
  function at the copy's line; a CPU profile of a loop through test/codegen/ladders's subroutine
  shows `pick main.xo:28` once under each drop (2026-11-14).
- Compile time: one pass over each function's lines and a map of run texts; nothing for a function
  without `ldr x0, [sp, #`.

## Results (2026-11-14)

- xo's own build, darwin/arm64, the same xo with the pass on and XO_PASSES=-ladders (both with the
  backend cache off): __text 37,283,708 to 35,456,348 bytes (-4.9%; 456 K instructions). Exact
  repeats of 16 or more pairs left: 20 pairs (from 219,527); repeats under 16 pairs: 29,107 pairs,
  unchanged (LADDER_MIN). build_native's repeats: 147,124 pairs to 1,093.
- A first build skipped far frames (build_native's among them): -1.6%.
- Tests: test/codegen/ladders (one `_ld0` subroutine, every arm calling it), compiler/debug
  ladder_test.xo (the fold and the frames it must keep), the arm64 and LLVM codegen patterns (48
  pass), compiler/debug and compiler/arm64 unit tests (103 pass), TestPassesListed.
- The --ab trio and aiplatform's __text: in the push's commit message.

## Steps

0. Measure xo's own amd64 binary (linux/amd64 host) with ladders-bin.py's x86 form; record it here.
1. (Done) compiler/arm64 ladders.xo, the folds of point 3, the tests above.
2. (Done in the push) gate as a codegen change: codegen patterns, fp, the self build, the --ab trio
   (HTTP CRUD server, alloc bench, xo's self build) on linux/arm64 host 1.
3. x86, if step 0 shows more than 2% of xo's amd64 text in repeats.

## Consequences

- An outlined ladder's pcs have no line; shown frames fold them (point 3), a raw pc lookup of one
  (lldb, addr2line) names the function with no line.
- The line at a `bl` to a subroutine is the run's last row (its rows all land on the `bl`).
- Splitting build_native is not done: it would not remove the copies.

## Alternatives

- Split build_native (the lead's question): moves the copies, saves no instructions.
- Release in SSA through one shared exit block per scope (extend reltail to exits with different
  targets: a tail that ends by jumping back to a continuation needs an indirect jump or a
  continuation index, which the SSA lacks): the subroutine is that, on the machine level.
- A per-function release helper called with a slot bitmap: data dependent, slower per release.
- Machine outlining across functions (LLVM's MachineOutliner): the slots differ between functions,
  so the ladders do not match across them.
