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

## Stage 2's shape: the arm64 MIR retargeted (lead, 2026-10-17)

The arm64 backend's front half builds MIR whose plain instructions are
arm64 assembly templates over virtual registers (isel.xo, calls.xo,
closures.xo, rc.xo, byref.xo, and the MIR passes from dce to
shrink_split). A corpus of every template the goldens, bench/perf and
selfhost/xo produce (2026-10-17: 15.2 million instructions) has 319
shapes once immediates and symbols are normalized, over about 60
mnemonics. amd64 shares that front half and adds a pass, retarget,
between shrink_split and regalloc, which rewrites each template as x86:

- a two operand form is a copy into the definition's vreg (a move the
  allocator coalesces when the source dies) and a read modify write of
  it; a spilled tied operand reloads and stores through one register;
- instructions with fixed registers (div and idiv in rax:rdx, shifts by
  cl) name them in Inst.clobbers, with r11 the emitter's scratch (as x16
  and x17 on arm64);
- conditions map by the kind of the flag producer: arm64's carry after a
  subtraction is the complement of x86's borrow, so hs and lo after cmp
  are ae and b, while cs after adds is b; a float compare maps to
  ucomisd with the operands ordered so one jcc tests an ordered
  relation, and eq and ne of floats test PF too;
- the shapes a text rewrite cannot carry are written as x86 directly by
  the front half under the target test: the load and store exclusive
  loops (lock xadd, lock cmpxchg, xchg), thread locals (%fs), the
  NEON byte searches (SSE2 pcmpeqb and pmovmskb), popcount, and the
  crypto intrinsics of stage 3.

x86 also gets its register sets in regalloc (SysV: rax, rcx, rdx, rsi,
rdi, r8 to r10 caller saved, rbx and r12 to r15 callee saved, rbp the
frame pointer, r11 scratch; xmm0 to xmm15, xmm15 scratch) and its own
emission (frames, spills, parallel moves, branches, calls, fault
stubs, CFI).

Gates (lead's conditions): the arm64 output stays byte identical, so
TestGoldenAsm and the arm64 fixed point pass on every change to the
shared front half. Once amd64 runs the goldens, bench/perf measures it
against LLVM on linux/amd64; where the translation costs (memory
operands, addressing modes, lea, flags), native x86 selection replaces
it function by function, and the gap per workload is recorded in
bench/results.

Layout and the split:
- stage 2a: the x86 files live in selfhost/arm64 (x86_*.xo), G carries
  the target ISA, and selfhost/xo's `--backend=amd64` calls the same
  generate entry;
- stage 2b, the split, once amd64 runs the 180 goldens on linux/amd64:
  selfhost/mir (the shared front half: G, the MIR, isel, calls,
  closures, rc, byref, inline, the MIR passes, regalloc's core),
  selfhost/arm64 (arm64 emission, its register sets, its text
  peepholes) and selfhost/amd64 (retarget, x86 emission, its register
  sets). The split is one mechanical change, checked by arm64 assembly
  byte identical over the goldens and selfhost/xo, and the fixed point.

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

- Stage 1 landed (Xo Lang 2, 2026-10-17): selfhost/macho enc_amd64.xo
  (the encoder) and asm_amd64.xo (the assembler, with amd64 paths in
  asm.xo, elf.xo and cfi.xo); `macho asm --amd64`, and
  `assemble_elf_amd64` for xo1. Tests, all against clang's integrated
  assembler (scripts/amd64-enc-goldens.py regenerates them):
  - 1,953 instruction forms (the emitter's set and stage 3's crypto
    set) encode to clang's bytes (enc_amd64_test.xo);
  - 3,192 forms of LLVM's own x86_64 output for 42 goldens (symbol free
    lines, up to 30 per mnemonic) likewise (enc_amd64_survey_test.xo);
  - whole files under selfhost/macho/testdata/amd64 give clang's ELF
    object byte for byte (asm_amd64_test.xo: a frame with CFI, calls,
    GOTPCREL, TLS, numeric labels; relaxation at the rel8 boundary; a
    jump table, a personality and LSDA, SSE and crypto);
  - the 42 LLVM-generated programs (their `.addrsig` lines removed,
    LLVM's address significance table being out of the subset) assemble
    to clang's objects byte for byte.
  LLVM's choices the assembler matches are in
  gotchas/amd64-encoder-llvm-choices.md.

## Alternatives

- Keep LLVM on x86_64: clang on every x86_64 machine, and no control of
  its code shape (the reason for the arm64 backend).
- Emit machine code directly from isel (no text): faster builds, but no
  cc oracle and a second path for every directive; the arm64 path's text
  has been cheap (its assembly is about a tenth of a build).
