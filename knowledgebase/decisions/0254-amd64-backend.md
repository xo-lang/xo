# 0254: Xo's own x86_64 backend (selfhost/amd64)

Status: Proposed (claimed 2026-10-17 by xo-fa; the lead's milestone
"native x86_64 without clang", with Xo Lang 2)

## Context

On arm64 hosts Xo builds with its own backend, assembler and linkers (no
C toolchain; decisions 0182, 0243, 0248). On x86_64 the default backend
is LLVM, so every x86_64 build needs clang. The arm64 backend
(selfhost/arm64: SSA to a machine IR, isel, regalloc, frames, text
assembly) and selfhost/macho (a text assembler and the Mach-O, ELF and
COFF writers and linkers) are the shape to follow.

## Decision (proposed)

A selfhost/amd64 backend with the arm64 backend's structure, emitting
GNU assembler text in AT&T syntax (what cc and clang assemble), and an
x86-64 assembler in selfhost/macho that encodes that text into the
object writers that exist. Text in between keeps cc as a fallback and as
the oracle: the same text assembled by clang and by Xo must give the same
bytes.

Stages and owners:

1. Encoder (Xo Lang 2): selfhost/macho asm_amd64.xo: the instruction set
   the emitter uses (integer, SSE2 scalar float, the BMI2/ADX, AES-NI,
   SHA-NI and PCLMULQDQ the crypto intrinsics use), REX and VEX
   prefixes, ModRM and SIB, rip relative operands, the relocations of
   ELF (R_X86_64_PC32, PLT32, GOTPCREL(X), TPOFF32), Mach-O and COFF.
   Golden tests: each form against llvm-mc's encoding.
2. Machine IR, isel, regalloc, frames, calls (xo-fa):
   selfhost/amd64 mir, isel (SSA to x86 forms: two operand instructions,
   memory operands, flags), regalloc (the arm64 one's linear scan with
   x86's register classes and the fixed registers of div, shifts and
   calls), frames (rbp chains, the SysV and Windows x64 ABIs, CFI), the
   runtime's intrinsics (rt.* that the arm64 emitter lowers: atomics,
   stack switch, thread locals, unwind save and resume), the fault and
   check stubs.
3. Crypto intrinsics on x86_64 (Xo Lang 2): cpu.adc chains (adc,
   adcx/adox, mulx with a fallback), SHA-256 (SHA-NI), AES rounds
   (AES-NI), carry-less multiply (PCLMULQDQ), as the arm64 emitter's.
4. ELF first: elflink's x86_64 relocations and its static mode (decision
   0243) for linux/amd64; then Mach-O (darwin/amd64, the Intel Mac) and
   PE/COFF (windows/amd64).
5. The default: x86_64 hosts default to selfhost/amd64 once the gates
   below pass; LLVM stays as --backend=llvm.

Verification: the 180 goldens on linux/amd64 in Docker (emulated, about
3x slower), the amd64 backend's asm pattern tests, the encoder's llvm-mc
goldens, progen across backends; CI on x86_64 runners when CI is back.
The self build's fixed point on x86_64 is the last gate before the
default changes.

## The interface between the emitter and the encoder (agreed 2026-10-17)

The emitter's output is one text file per module (or per part, as
arm64's generate_parallel), GNU assembler syntax, AT&T operand order.
The encoder takes that text and gives the object writers' sections,
symbols and relocations, as selfhost/macho's arm64 assembler does. The
encoder accepts exactly this subset; anything else is an error with the
line, never a guess. The same text must assemble with
`clang -c -target x86_64-unknown-linux-gnu` (and the Mach-O and COFF
triples) to the same bytes, which is the encoder's test.

Instructions:
- Mnemonics carry their size suffix always (movq, movl, movw, movb,
  addq, cmpl, movzbl, movsbq, movslq, ...); SSE scalar forms (movsd,
  addsd, cvtsi2sdq, ucomisd, ...), and the crypto set of stage 3.
- Registers %rax to %r15 and their 32, 16, 8 bit names (%eax, %r8d,
  %ax, %r8w, %al, %r8b, %sil, %dil, %bpl, %spl), %xmm0 to %xmm15,
  %rip only in memory operands.
- Immediates `$n` (decimal or 0x hex, signed), `$sym` and `$sym+n`.
- Memory: `disp(base)`, `disp(base,index,scale)`, `(,index,scale)`
  with disp a number, `sym`, `sym+n`, or `sym-sym2` within a section;
  `sym(%rip)`, `sym+n(%rip)`; `%fs:` and `%gs:` prefixes.
- Branches: `jmp L`, `jCC L` (the jcc set by condition code), `call sym`,
  `jmp sym` (a tail call), `call *%reg`, `call *disp(%reg)`,
  `jmp *%reg` (jump tables). Local jumps relax: the shortest form that
  reaches, iterated to a fixed point (clang's choice, so the bytes
  match).
- One instruction per line; a `#` starts a comment to the end of the
  line; labels end in `:`; local labels start with `.L` (ELF, COFF) or
  `L` (Mach-O).

Symbol references and their relocations:
- ELF: `sym(%rip)` R_X86_64_PC32; `call sym` and `jmp sym` to a
  symbol outside the section R_X86_64_PLT32; `sym@GOTPCREL(%rip)` in a
  movq R_X86_64_REX_GOTPCRELX; `%fs:sym@TPOFF` R_X86_64_TPOFF32 (local
  exec thread locals, static links); `.quad sym` R_X86_64_64; `.long
  sym-.` R_X86_64_PC32.
- Mach-O: `_sym(%rip)` X86_64_RELOC_SIGNED, branches
  X86_64_RELOC_BRANCH, `_sym@GOTPCREL(%rip)` X86_64_RELOC_GOT_LOAD,
  `_sym@TLVP(%rip)` X86_64_RELOC_TLV, `.quad _sym` X86_64_RELOC_UNSIGNED.
- COFF: `sym(%rip)` IMAGE_REL_AMD64_REL32, `.quad sym`
  IMAGE_REL_AMD64_ADDR64, `.secrel32` for the TLS index (to be fixed
  with the PE stage).

Directives (the arm64 emitter's set): .text, .data, .section with the
ELF flags and types it prints (.rodata, .rodata.str1.1, .tdata,
.gcc_except_table, .note.GNU-stack, .data.DW.ref.*), .globl, .hidden,
.weak, .type, .size, .p2align, .byte, .short, .long, .quad, .ascii,
.asciz, .zero, .uleb128, .sleb128, .file and .loc (decision 0239's
lines), and .cfi_startproc, .cfi_endproc, .cfi_def_cfa,
.cfi_def_cfa_offset, .cfi_offset, .cfi_personality, .cfi_lsda (the
encoder writes .eh_frame from them, as the arm64 assembler does).

Details the encoder matches (Xo Lang 2's review, 2026-10-17), so its
bytes stay clang's:
- The shortest immediate: `addq $1, %rax` is the imm8 form (83 /0),
  `$300` imm32; `movq $n` with n outside int32 assembles as clang does
  (movabsq); the emitter writes `movabsq` for 64 bit constants anyway.
- `.p2align` in text pads with LLVM's long NOPs (0f 1f ... up to 15
  bytes, then repeated), not `90`s.
- A `call`/`jmp` to a global symbol defined in the same section still
  gets R_X86_64_PLT32 (it can be preempted), to a local label none.
- TLS in a dynamically linked ELF (XO_NOLIBC=0): initial exec,
  `movq sym@GOTTPOFF(%rip), %reg` R_X86_64_GOTTPOFF, then `%fs:(%reg)`.
- CFI also takes `.cfi_def_cfa_register %rbp` (an rbp frame after `movq
  %rsp, %rbp`), so the emitter may write clang's prologue CFI.
- The crypto set named: `adcxq`, `adoxq`, `mulxq` (BMI2/ADX),
  `sha256rnds2`, `sha256msg1`, `sha256msg2`, `aesenc`, `aesenclast`,
  `aesdec`, `aesdeclast`, `aesimc`, `aeskeygenassist`, `pclmulqdq`,
  with the SSE moves they need (`movdqu`, `movdqa`, `pshufd`,
  `pshufb`, `pxor`, `movq %xmm, %reg` and back, `pinsrq`/`pextrq` from
  SSE4.1).

What the encoder does not need: no macros, no .if, no expressions
beyond `sym+n`, `sym-sym2` and `sym-.`, no Intel syntax.

## Consequences

To be filled in as stages land.

## Alternatives

- Keep LLVM on x86_64: clang on every x86_64 machine, and no control of
  its code shape (the reason for the arm64 backend).
- Emit machine code directly from isel (no text): faster builds, but no
  cc oracle and a second path for every directive; the arm64 path's text
  has been cheap (its assembly is about a tenth of a build).
