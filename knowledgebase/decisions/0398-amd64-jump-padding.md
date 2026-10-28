# 0398: amd64 jump padding: no jump crosses or ends on a 32 byte boundary

Status: Accepted (2026-10-28, lead; xo-lang-6). Extends decision 0254
(the amd64 backend and Xo's x86-64 assembler) and 0075 (code
placement). Implemented in compiler/macho asm_amd64.xo (`x86_jump_pad`).
Evidence: bench/results/sort1m-x86-2026-10-28.md.

## Context

Intel's JCC erratum (SKX102 and its siblings, the microcode of
2019-11): on Skylake, Kaby Lake, Coffee Lake, Cascade Lake and Comet
Lake cores, the fix stops the decoded uop cache from holding a jump
(jcc, jmp, call, ret, direct or indirect) that crosses a 32 byte
boundary or ends on one, or the cmp, test, add, sub, and, inc or dec a
jcc macro-fuses with. Such a jump is fed by the legacy decoder every
time, and a hot loop whose branch lands there runs slower by a fraction
that depends only on where the linker put it. Ice Lake and later cores
have no such erratum; AMD cores have none.

The Intel tester's sort1M (List[Int].sort, Go's pdqsort ported in
3b0f706ac) ran 1.23x Go on an i7-9750H, a Coffee Lake, at one
placement, with 7.5 percent fewer instructions than Go. The same
program at 32 placements (`XO_ARM64_PAD=0..31`, a one byte nop each on
x86) ran 73 to 90 ms against Go's 67, and the ranking is a function of
one number: the offset modulo 32 of pdq_partition's two inner scan
loops. Offsets where no fused cmp+jcc of either loop touched a boundary
ran 73 to 80 ms; offsets where the bounds check's cmp+jae or the exit's
test+je straddled one ran 81 to 90. Moving only pdq_partition (a
throwaway per function pad) gave the same ordering. Go's assembler has
padded every amd64 jump this way since Go 1.15 (cmd/internal/obj/x86
padJump and fusedJump), which is why Go's sort does not move.

An earlier run with clang's `-mbranches-within-32B-boundaries` (the
2026-10-27 results file) ruled the erratum out; that binary was never
padded, because xo assembles the native amd64 path with its own
assembler and the flag reached nothing
(gotchas/amd64-clang-flags-do-nothing.md).

## Decision

- Xo's x86-64 assembler pads every jump, and every macro-fusable pair,
  that would cross or end on a 32 byte boundary with LLVM's long NOPs
  before it (before the pair), so it sits inside one 32 byte block. A
  fusable instruction is the cmp, test, add, sub, and, inc or dec on
  the source line just before the jcc, in the same section, with no
  fixup; a label on it moves with it, so a loop entered through that
  label never runs the NOPs (the NOPs run once, on the fall through).
  A call, a ret and an indirect jmp or call are padded alone.
- The padding counts offsets from the section start, so every amd64
  code section is 32 byte aligned (`align` 5), which the Mach-O, ELF
  and COFF writers and Xo's linkers keep.
- It is on for every amd64 object Xo assembles (darwin, linux, windows
  through coff.xo), on every CPU: the cost is code size, measured
  below, and it needs no CPU detection at compile time, as Go's. The
  `assemble_elf_amd64_raw` and `assemble_macho_amd64_raw` entry points
  skip it for the encoder's clang goldens, which compare bytes with
  `clang -c`.
- The relaxation loop (short to long branches, never back) re-parses
  the source with the padding recomputed from each pass's offsets, so
  labels, CFI and the padding are those of the final layout.
- The Windows path that assembles with clang (win_amd64_link, when
  used) gets no padding from this; its flag is a follow up for Lang 2.
- The pad sweep is the gate for a placement claim: `XO_ARM64_PAD=0..31`
  on x86 (a 32 byte window), judged across placements, never at one
  (decision 0075; knowledgebase/gotchas/arm64-indirect-dispatch-placement.md).

## Consequences

- sort1M on the i7-9750H: 69 to 72 ms over the same 32 placements (Go 67), every one under the unpadded best of 73; interp 160 to 128 ms (bench/results/sort1m-x86-2026-10-28.md).
- Code size: xo's own amd64 `__text` 106,296,832 to 109,215,304 bytes
  (+2.75 percent; the file +1.85 percent), more than Go's fraction of a
  percent because Xo's code has a fused cmp+jae bounds check at most
  list reads. A follow up can pad only loops: a jcc whose target label
  is already defined is a back edge, and the jumps between the target
  and the back edge are the loop's; everything else stays unpadded.
  Measure it against the 32 placement sweep before switching.
- Every amd64 object changes bytes (NOPs, section alignment): the amd64
  cross build identity against the parent is a known change of this
  push; the arm64 output is byte identical (the assembler is x86 only).
- Where a hot jump straddled, 10 to 23 percent on the affected Intel
  cores; nothing measurable where none did, or on Ice Lake and later
  and AMD (the NOPs are never in a loop body unless a fused pair's
  label is not the loop head).
- The clang goldens keep comparing byte for byte through the raw entry
  points; the padded path has its own tests (asm_jpad_test.xo);
  compiler/macho's 78 tests pass; the Docker amd64 golden sample ran 48
  of 48 with the padded assembler.
- Pending: the same programs on a CPU without the erratum
  (scripts/ci/jump-pad-timing.sh on the xo-amd64-big runner, a Sapphire
  Rapids Xeon; one owner approved CI run), to confirm the padding costs
  nothing there.

## Alternatives

- Prefix padding (LLVM's `-mbranches-within-32B-boundaries` default
  mode adds segment prefixes to earlier instructions of the block, no
  extra uops): fewer bytes executed, but the encoder would re-encode
  earlier lines whose bytes are memoized by text; NOPs before the jump
  are Go's choice and run only on fall through when the label moves.
- Aligning loop heads (`.p2align 5`): one fixed placement, 10 percent
  worse on this program (the 2026-10-27 file), and decision 0075's
  result on arm64.
- Function alignment to 32 with per function offsets (Go's FuncAlign):
  robust to a linker that drops or reorders functions; costs about 16
  bytes a function. Not needed while Xo's linkers keep section layout;
  the dead stripping by symbol of 0388 stage 4 must keep each kept
  function's offset modulo 32 (or align it to 32) for the padding to
  hold: noted for xo-lang-4.
- A CPU check at run time: the padding is a static layout; Go pads
  everywhere too.
