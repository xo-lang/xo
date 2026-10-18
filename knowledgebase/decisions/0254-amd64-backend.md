# 0254: Xo's own x86_64 backend (selfhost/amd64)

Status: Accepted (lead, 2026-10-17); stages 1, 2a and 4 implemented
(xo-fa and Xo Lang 2), stage 3 (crypto intrinsics) and 2b (the module
split) next

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

## Stage 6 (Proposed, xo-fa, 2026-10-18): darwin/amd64 and windows/amd64

The owner asked (2026-10-18, through the lead) for a plan and size for
the amd64 backend on Windows and on macOS x86_64, ranked. Today both
build through LLVM only: the amd64 backend emits ELF and links with
elflink (stages 2a and 4).

What exists to build on: the x86_64 encoder and ELF assembler
(selfhost/macho enc_amd64.xo 1,479 lines, asm_amd64.xo 333); elflink's
x86_64 links (1,992 lines, both arches); the Mach-O assembler, linker and
compact unwind writer, arm64 only (asm.xo 1,340, link.xo 1,359,
unwind.xo 291, obj.xo 235: CPU_TYPE_ARM64 and the ARM64_RELOC_* set
hard coded); the Windows runtime in Xo that LLVM builds use
(std/native/llrt windows.xo 1,335, sys_windows.xo 302, and unwind.xo's
Windows path through RtlLookupFunctionEntry and RtlVirtualUnwind, which
needs .pdata and .xdata for every Xo function); the winxc cross
toolchain (lld-link, the xwin sysroot) and Wine under box64 in Docker
for running Windows programs; the owner's Intel MacBook for darwin/amd64
runs (a real CPU: SHA-NI, AES-NI, native speed).

### 6a. darwin/amd64 (recommended first)

- The calling convention is System V, as Linux: x86_call_locs, the
  callee saved set and the frame stay. The emitter's changes are the
  Mach-O spellings: `_sym` names, `_sym@GOTPCREL(%rip)` for library
  data and weak references, thread locals through `_xo_tlocal@TLVP(%rip)`
  and a call of the descriptor's thunk (the arm64 darwin form, x86
  registers: the thunk preserves all but rax and rdi), `.subsections_via_symbols`.
- Assembler: Mach-O x86_64 objects (CPU_TYPE_X86_64, the relocations
  X86_64_RELOC_SIGNED(_1/_2/_4), BRANCH, GOT_LOAD, GOT, TLV, UNSIGNED,
  SUBTRACTOR), sharing asm.xo's directive and section layer.
- Linker: link.xo's x86_64 path: stubs (`jmp *GOT(%rip)`) and lazy
  binding or chained fixups as for arm64, GOT and TLV relocations, the
  load commands with CPU_TYPE_X86_64; no code signature is required on
  Intel (keep the ad hoc one for parity).
- Unwind: compact unwind for x86_64 (UNWIND_X86_64_MODE_RBP_FRAME with
  the pushed registers' encoding; the emitter's frames are rbp frames, so
  every function fits; frameless leaves use MODE_STACK_IMMD), from the
  CFI the emitter already writes (cfi.xo reads it for arm64).
- Size: about 1,500 to 2,500 lines (assembler relocations 300 to 500,
  linker 600 to 1,000, compact unwind 200 to 300, emitter 200 to 400,
  tests). Two to four days of one session.
- Verification: the 190 goldens and the fixed point on the owner's Intel
  MacBook (bench/results intel runs; xo cross built on the arm64 Mac and
  copied over), byte comparison of the assembler against clang's
  `-target x86_64-apple-macos` as stage 1 did for ELF.

### 6b. windows/amd64

- Calling convention (Win64): arguments in rcx, rdx, r8, r9 (each
  position one register of either class: xmm0 to xmm3 for a Float in
  that position), 32 bytes of shadow space above the return address that
  every caller reserves, results in rax (and xmm0), callee saved rbx,
  rbp, rdi, rsi, rsp, r12 to r15 and xmm6 to xmm15 (SysV saves no xmm).
  Xo's own calls can keep the SysV like internal convention (as the
  arm64 backend keeps one ABI) with Win64 only at the boundary: calls of
  platform functions (sys_windows.xo's kernel32, ws2_32, bcrypt), the
  exported entry and callbacks the OS makes (the vectored exception
  handler, thread starts). regalloc gains the Win64 callee saved xmm set
  where a function calls the platform.
- Unwind and faults: .pdata (RUNTIME_FUNCTION per function) and .xdata
  (UNWIND_INFO: UWOP_PUSH_NONVOL per pushed register, UWOP_SET_FPREG for
  rbp, UWOP_ALLOC_SMALL/LARGE, the chained info for shrink wrapping's
  region), so RtlVirtualUnwind walks Xo frames for the fault unwinder
  and the profiler. Leaf functions without a frame need no entry.
- Thread locals: the TLS index (`_tls_index`, `__tls_array` at gs:0x58),
  `.tls$` sections and IMAGE_REL_AMD64_SECREL relocations, as clang
  writes them.
- Object and link: COFF objects (IMAGE_FILE_MACHINE_AMD64, the
  IMAGE_REL_AMD64_REL32(_1.._5), ADDR64, ADDR32NB for .pdata, SECREL,
  SECTION relocations, COMDAT none), linked by lld-link through the
  winxc toolchain first (the import libraries from the xwin sysroot);
  Xo's own PE linker (import tables, base relocations, the TLS directory,
  the exception directory) as a later step, about the size of elflink.
- Size: about 4,000 to 6,000 lines with lld-link (ABI boundary 500 to
  800, .pdata/.xdata 500 to 800, COFF writer and relocations 800 to
  1,200, TLS 200 to 300, the runtime's Win64 entry points 300 to 500,
  tests), plus 2,500 to 3,500 for an own PE linker. One to two weeks of
  one session.
- Verification: the goldens under Wine with box64 (structured exceptions
  work there; network and file system goldens have known Wine gaps,
  toolchain.md), and on a Windows EC2 instance or CI runner once CI is
  back (remote-windows.sh; agents do not start it themselves).

### Order

The lead suggested Windows first for its audience. The evidence for
macOS first: (1) it is about a third of the work, because the System V
convention and the rbp frames carry over unchanged; (2) it can be
verified on real hardware now (the owner's Intel MacBook runs the
goldens and the fixed point at native speed), while Windows has only
Wine under box64 here, where wall times mean nothing and several
goldens fail for Wine reasons; (3) it builds the second object format
and the non ELF link path that Windows needs too (a relocation model
per format, unwind tables from the same CFI), so Windows starts from a
generalized assembler and linker. The case for Windows first is the
audience, and that darwin/amd64 is a shrinking platform (Apple ships no
Intel Macs). Proposed: 6a, then 6b with lld-link, then the own PE
linker; reverse 6a and 6b if the owner weighs the audience above the
verification gap.

## Consequences

Stage 2a landed 2026-10-17: `xo build --backend=amd64` builds for
linux/amd64 (from any host; on a linux/amd64 host it links). All 182
goldens pass on linux/amd64 (Docker, emulated), and with no C toolchain
(XO_CC and XO_CLANG set to /bin/false): a no-libc program is assembled
by selfhost/macho's x86-64 assembler (stage 1) and linked by elflink's
static x86-64 link (stage 4); the arm64 output is byte identical (the
arm64 assembly of every golden and selfhost/xo for darwin and linux, 366
builds, against the merge base). What it took beyond the plan, in
knowledgebase/gotchas/amd64-retarget-semantic-gaps.md:

- the x86 sdiv branches around idiv for 0 and -1 (arm64's sdiv does not
  trap, and the front half speculates divisions);
- a spilled source operand is used from its slot where x86 takes a
  memory source, and when no register is free at an instruction one is
  borrowed through the red zone below rsp;
- carry chains are cut into one instruction per link (x86_split), as one
  template they needed more registers than x86 has;
- memory to memory parallel moves are push and pop (r11 may hold a move
  cycle's value);
- shrink.xo's tail call test takes the arguments' locations (call_locs),
  which on x86 differ from the results';
- the SSA builder lowers only cpu.xo's carry arithmetic for amd64
  (Builder.soft_cpu); AES, SHA-256 and carry-less multiply run their Xo
  bodies until stage 3.

Not yet: darwin/amd64 and windows/amd64 (Mach-O and PE, stage 4's
rest), DWARF variable locations (debug_info is off for
amd64), the per-function cache, native x86 selection where the
translation costs (the bench/perf comparison with LLVM, the lead's third
condition). Shrink wrapping (decision 0224) runs for amd64 since
2026-10-17.

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
- Stage 4, static links (Xo Lang 2, 2026-10-17): selfhost/macho elflink
  links x86-64 objects in its static mode (decision 0243), `macho
  elflink --static`: R_X86_64_64, PC32, PLT32, PC64, GOTPCREL(X) through
  GOT slots, 32 and 32S, TPOFF32 with TLS variant 2 (the block ends at
  the thread pointer), GOTTPOFF relaxed to local exec (the mov or add
  load becomes an immediate), and calls of weak undefined symbols a
  5 byte nop or a ret. End to end: 30 goldens compiled by the LLVM
  backend in Docker (linux/amd64, `clang -S -O2` with local exec thread
  locals), assembled by Xo's assembler and linked by elflink on the Mac,
  run in Docker linux/amd64: 29 of 29 linked pass their want.txt
  (tz_table needs the build's generated zone table object, outside the
  dump).
- Stage 4, glibc links (Xo Lang 2, 2026-10-17): elflink's dynamic mode
  takes x86-64 objects too: crt1's `_start` for x86-64, 16 byte stubs
  (`jmp *slot(%rip)`) through GLOB_DAT GOT slots (bound at load,
  BIND_NOW), R_X86_64_64 to an import as a dynamic relocation, the
  loader /lib64/ld-linux-x86-64.so.2, versions from glibc 2.31 on
  linux/amd64 (selfhost/macho/glibc_amd64.xo, base GLIBC_2.2.5;
  `scripts/elf-glibc-versions.py --amd64`), and libc_nonshared's stat
  wrappers with version 1. 12 LLVM-compiled goldens of the glibc build
  linked this way pass on linux/amd64 (Debian bullseye's loader).
- The amd64 corpus through Xo's own tools (Xo Lang 2, 2026-10-17, wip/
  fa-amd64 270879e8): `xo asm --backend=amd64 --targets=linux/amd64` of
  all 182 goldens, `macho asm --amd64` and `macho elflink --static` on
  the Mac, run in Docker linux/amd64: 181 link (tz_table needs the
  build's zone table object) and all 181 pass.
- Stage 3, crypto (Xo Lang 2, 2026-10-17): the carry intrinsics were
  already lowered (the arm64 adcs and cset templates retargeted, cut by
  x86_split). Added under `if g.x86` in selfhost/arm64/calls.xo: clmul_lo
  and clmul_hi as PCLMULQDQ (one for a pair), aes_round as pxor then
  aesenc with a zero key (arm64's aese and aesmc add the key first),
  aes_last as pxor then aesenclast, aes_enc10/12/14 as the standard
  block (round keys loaded with movdqu, no alignment assumed), and
  cpuid1_ecx as CPUID leaf 1 (rbx kept in r11). selfhost/ssa cpu_call
  lowers every cpu.xo intrinsic on amd64 but sha256_block, so lowered()
  is 1 and hw() reads CPUID (AES and CLMUL; 3 in Docker linux/amd64 on
  the Mac's Rosetta). SHA-256's compression keeps its body: SHA-NI needs
  CPUID leaf 7, a new runtime primitive (the owner's call). AES-GCM
  sealing 32 MiB in Docker: 571 ms with the bodies, 38 ms lowered.
- The first native x86 selection (Xo Lang 2, 2026-10-17; the ranking in
  bench/results/amd64-vs-llvm-2026-10-17.md, split with Xo lang 5): a
  checked product is imul and jo (`x86_mulo`), a load or store at i + c
  takes c as its displacement (calls.xo x86_disp, xmem's `[base, index,
  #disp]`), and and, or, xor take any 32 bit immediate (a complemented
  constant too); arm64's output is unchanged. collatz -12.6%, iface
  -23.6%, hof -18.3%.
- SHA-256 on x86-64, approved by the owner on 2026-10-17 (the new
  runtime primitive rt.cpuid7_ebx included) and done the same day (Xo
  Lang 2): cpu.xo cpuid7_ebx and hw()'s HW_SHA256 from its bit 29; the
  SHA-NI sequence (ssa.sha256_ni_lines, the Linux kernel's sha256_ni
  form) in the Xo backend (calls.xo, xmm0 to xmm10 saved around it) and
  in LLVM (inline asm, clobbered); the assembler's palignr; a Go
  compression for crypto_op's cpu.sha256_block and a cpu.hw op; the
  golden cpu_sha256 (the "abc" digest and 50 blocks, lowered against
  the body where hw() has SHA-256). Under qemu-x86_64 -cpu max hw() is
  11 and both lowerings agree with the body; with -cpu Nehalem and under
  Rosetta hw() is 0 and the body runs. The plan as proposed: (1) `rt.cpuid7_ebx`, CPUID
  leaf 7 subleaf 0's EBX, beside rt.cpuid1_ecx (cpu.xo `cpuid7_ebx`, body
  0; on x86 `movq %rbx, %r11; movl $7, %eax; xorl %ecx, %ecx; cpuid;
  movl %ebx, $wd0; movq %r11, %rbx`), and detect() setting HW_SHA256 from
  its bit 29; (2) sha256_block's x86 template: the state packed into
  ABEF and CDGH (pshufd, palignr, pblendw from the eight 8 byte words),
  the message bytes swapped (pshufb with a constant mask), 16 groups of
  sha256rnds2 twice (xmm0 is its implicit W + K operand, so the template
  saves the allocator's xmm0) with sha256msg1, palignr and sha256msg2
  for the schedule, then the sums unpacked to the 8 byte words; (3) the
  ssa soft_cpu exclusion dropped. Testing: Docker on this Mac (Rosetta)
  reports no sha_ni, so cpu_prims' sha256 rows (the lowered result
  against the body) run on a real x86-64 CPU with SHA-NI (Intel since
  Goldmont and Ice Lake, AMD since Zen) or under qemu-x86_64 with
  `-cpu max`, with a forced HW_SHA256 for the test only.

- Keep LLVM on x86_64: clang on every x86_64 machine, and no control of
  its code shape (the reason for the arm64 backend).
- Emit machine code directly from isel (no text): faster builds, but no
  cc oracle and a second path for every directive; the arm64 path's text
  has been cheap (its assembly is about a tenth of a build).
