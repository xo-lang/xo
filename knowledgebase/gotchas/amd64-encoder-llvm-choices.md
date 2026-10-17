# The x86-64 encoder must make LLVM's choices, or the bytes differ

Status: Observed (2026-10-17, xo-4b, decision 0254 stage 1).

## Symptom

An x86-64 object that disassembles the same as clang's still differs
from it byte for byte: a different encoding of the same instruction, a
relocation type, padding, or a header field.

## Cause and what the assembler does

Each of these was a difference against `clang -c -target
x86_64-unknown-linux-gnu`, now matched (compiler/macho enc_amd64.xo,
asm_amd64.xo):

- **Encodings**: the imm8 form when the value fits (83 /n), the
  accumulator forms for imm32 (05 id for `addq $300, %rax`); `movq $n`
  with n outside int32 is movabsq; `xchg` between two registers puts
  the destination in ModRM's reg field; `lock` comes after the operand
  size prefix (66 f0); `rep bsf` is LLVM's spelling of tzcnt.
- **Padding**: `.p2align P` in code pads with LLVM's long NOPs, at most
  10 bytes each (the generic x86-64 CPU); `.p2align P, 0x90` (an
  explicit fill) pads with 0x90 bytes.
- **Branches**: a jmp or jcc to a label starts short and becomes rel32
  when it does not reach or needs a relocation (a global or weak target,
  another section), iterated to a fixed point; `call` is always rel32,
  PLT32 when relocated.
- **Relocations**: a GOTPCREL is GOTPCRELX (REX_ with a REX prefix) only
  for the instructions the linker may relax (mov loads, indirect call
  and jmp, test, and add to cmp reg <- mem); any other is plain
  GOTPCREL. GOT and TLS relocations name their symbol; others to a local
  symbol use the section symbol. `.long A-B` with B in the data section
  (a jump table) is PC32 with the field's distance from B in the addend.
- **Headers**: `.eh_frame` is SHT_X86_64_UNWIND; EI_OSABI is 3 (GNU)
  when any section is SHF_GNU_RETAIN (flag `R`); the CIE has data
  alignment -8, return address column 16, and `def_cfa %rsp, 8;
  offset %rip, -8` as its initial instructions.

## How to find the next one

Assemble the same text with clang and with `macho asm --amd64`, then
compare section by section (sections, symbols, relocations and data); a
whole file matching a disassembly but not `cmp` is usually a header
field or padding.

- Every `.file` is its own STT_FILE symbol (Observed, 2026-10-17, the
  amd64 emitter's first text repeats `.file "prog.s"` per module): LLVM
  keeps duplicates and puts each before the locals created after its
  directive (the first before all of them). The assembler deduplicated
  them and listed them first, so every symbol index after the first
  local differed. Function sizes still differ from clang on purpose:
  `func_sizes` gives a function without `.size` its extent (valgrind,
  perf).
