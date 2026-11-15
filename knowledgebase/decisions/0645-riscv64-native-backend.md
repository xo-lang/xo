# 0645: A native linux/riscv64 backend

Status: Proposed (Xo Lang 1, 2026-11-15; owner priority through the lead,
2026-11-15: "a native linux/riscv64 backend is next"). Supersedes
Decision 2 of 0571 (no native backend at first) once accepted. The
owner's features-first rule applies: land the backend broadly, then the
evidence.

## Context

linux/riscv64 builds today only with `--backend=llvm` (decision 0571,
stages 1 to 3 done by 2026-11-10): every LLVM golden that needs no
network and std's 60 test modules pass under qemu-user on offload host
2. Every riscv64 build therefore needs clang, a glibc sysroot and
clang's build times, while arm64 and x86-64 build with Xo's own
backend, assemblers and linkers (decisions 0182, 0243, 0254, 0270).

What the native backends look like on main (2026-11-15):

| Part | Where | Lines | riscv64 |
|---|---|---|---|
| Shared front half: MIR as arm64 assembly templates over virtual registers, isel, calls, closures, rc, byref, inline, the MIR passes | compiler/arm64 (isel.xo 4,564, calls, closures, ...) | | shared, as x86 shares it |
| x86-64: the retarget pass (arm64 templates rewritten as x86), emission, runtime intrinsics, Windows | compiler/arm64 x86.xo 3,411, x86emit.xo 2,391, x86rt.xo 630, x86win.xo | about 7,000 | the shape to copy |
| Register allocation (linear scan over register classes) | compiler/arm64 regalloc.xo 1,636 | | riscv64's register sets |
| Encoders: text to bytes (decision 0274 kept the text interface) | compiler/macho enc.xo 556 (arm64), enc_amd64.xo 1,663, asm_amd64.xo 655 | | a new enc_riscv64.xo |
| ELF writer and linker: static and dynamic, GOT, TLS, islands | compiler/macho elf.xo, elflink.xo 2,661 (machines 183 and 62) | | machine 243 and its relocations |
| LLVM port: triple, ecall glue, the context switch, the preemption stub, the riscv64 unwinder step, sigcontext offsets, want_linux_riscv64.txt | 0571 | | the reference for every runtime detail |

RISC-V is closer to arm64 than x86 is: a load/store machine with three
operand instructions and 32 integer registers. Retargeting arm64
templates to riscv64 needs no two-operand rewrite. The differences that
cost work are:

- No condition flags. arm64's `cmp` with `b.cond`, `cset` and `csel`,
  and the `adds`/`adc` carry chains, become compare-and-branch
  (`beq`, `blt`, `bltu` and their inverses), `slt`/`sltu`/`seqz`/`snez`,
  and branches (or Zicond's `czero`, where present) for a select. Carry
  chains become `sltu` sequences, which stay constant time.
- Small immediates. ALU and load/store immediates are 12-bit signed;
  constants are built with `lui`, `addi` and shifts (or a constant pool).
  arm64's `movz`/`movk` sequences and its 9-bit and scaled 12-bit offsets
  map onto other limits.
- Addressing modes. Only base plus a 12-bit offset: no register plus
  register, no scaled index (except Zba's `sh1add` to `sh3add`), and no
  pre- or post-index. arm64's `ldp`/`stp` pairs become two instructions.
- Branch ranges. A conditional branch reaches about ±4 KiB and `jal`
  about ±1 MiB. Far branches need relaxation (an inverted branch around
  a `jal`), as arm64's far_tests_l handles `tbz`'s ±32 KiB. Calls use
  `auipc` plus `jalr` (R_RISCV_CALL_PLT) when the target may be far.
- Misaligned access. The ISA allows it, but many cores trap and emulate
  it in the kernel at about 100x the cost (Linux reports this through
  `riscv_hwprobe`). arm64 code, and std's word-at-a-time scans, assume
  cheap unaligned loads. The retarget must keep emitted accesses
  aligned, and std's word-at-a-time paths need an aligned form or a
  byte loop on riscv64.
- Division. Division by zero returns all ones and does not trap.
  Native arm64 already checks explicitly (`sdiv` returns 0), so this
  only means keeping the check.

## Decision (proposed)

1. **A third native target in compiler/arm64**, as x86 was added (0254
   stage 2a): riscv64 shares the front half and adds a pass `rv_retarget`
   between shrink_split and regalloc. The pass rewrites each arm64
   template as RV64 instructions. The shapes a text rewrite cannot carry
   (exclusive loads and stores, thread locals, the NEON byte searches,
   popcount, the crypto intrinsics, flag-based selects and carry chains)
   are emitted for riscv64 directly by the front half, under the target
   test. The files are rv.xo (retarget), rvemit.xo (frames, spills,
   parallel moves, branches, calls, fault stubs, CFI) and rvrt.xo (the
   rt.* intrinsics). `G` gains the riscv64 ISA, and `--backend=riscv64`
   calls the same generate entry. The 0254 stage 2b split (compiler/mir
   shared) stays separate and is not a prerequisite.

2. **Encoder: RV64GC, in compiler/macho enc_riscv64.xo**, from the text
   the emitter writes (GNU syntax), as enc.xo does for arm64.
   - Base: RV64I, M, A, F, D, Zicsr, Zifencei. That is "G", which Debian
     and Ubuntu (and Go's default `GORISCV64=rva20u64`) assume.
   - Compressed (C): the encoder emits the 16-bit form whenever the
     operands fit, as llvm-mc does with `+c`. This saves about a quarter
     of the code size. It comes in its own stage (Plan 4), because it
     makes instruction sizes variable, so branch sizing then iterates as
     amd64's jump padding does (0398).
   - Zba, Zbb (and later Zicond, Zbs, V): not in the baseline. A target
     level, `XO_RISCV64=rva20u64|rva22u64` (Go's GORISCV64 names; default
     rva20u64), lets codegen use Zba (`sh1add` to `sh3add`, `add.uw`)
     for indexing and Zbb (`clz`, `ctz`, `cpop`, `rev8`, `andn`, `orn`,
     `min`/`max`, `sext.b`/`.h`, `zext.h`, `rol`/`ror`) for the intrinsics
     that need them. There is no runtime dispatch in generated code; the
     runtime may dispatch a few hot routines on `riscv_hwprobe`. The
     level joins the binary and back-end cache keys, as the instruction
     selector did in d4e8220cf.
   - Oracle: every form must encode to llvm-mc's bytes
     (`-triple=riscv64 -mattr=+m,+a,+f,+d,+c -riscv-no-aliases`, with
     relaxation off). The text must also assemble with
     `clang -c --target=riscv64-unknown-linux-gnu -mno-relax` to the same
     bytes, which is the encoder's golden test (0254's interface rule:
     the encoder accepts exactly the emitter's subset, with an error for
     anything else).
   - No linker relaxation. Xo never emits R_RISCV_RELAX, so label
     differences inside a section resolve at assembly time and need no
     R_RISCV_ADD/SUB pairs (DWARF and `.eh_frame` included).

3. **Register allocation and the ABI: LP64D**, for Xo's own calls as
   well as C calls (`use c`), so one convention serves debuggers,
   profilers and the C boundary.
   - Arguments and results: a0 to a7 (and fa0 to fa7 for floats).
   - Caller saved: t0 to t6, a0 to a7; ft0 to ft11, fa0 to fa7.
   - Callee saved: s1 to s11, fs0 to fs11.
   - Fixed: s0 is the frame pointer (kept, for XO_PROF and the fault
     walk, as x29 on arm64); ra, sp; gp is unused (no gp relaxation);
     tp is the thread pointer.
   - Scratch: t5 and t6, as x16 and x17 are arm64's; t0 holds the
     `auipc`/`jalr` call scratch.
   - regalloc.xo gains the riscv64 class tables, as it did x86's.
   - 0151's by-address rule for structs of 256 bytes and up carries
     over. Within Xo code, the aggregate passing that d4e8220cf had to
     work around in LLVM does not arise: the native backend owns both
     sides of every call.

4. **ELF: the elflink and elf writers gain machine 243 (EM_RISCV).**
   - e_flags: EF_RISCV_RVC (once C is on) and
     EF_RISCV_FLOAT_ABI_DOUBLE (0x5 together).
   - Relocations: R_RISCV_64; R_RISCV_BRANCH and R_RISCV_JAL; R_RISCV_CALL_PLT;
     R_RISCV_PCREL_HI20 with PCREL_LO12_I and PCREL_LO12_S (the LO12
     relocation points at its HI20's label, a pairing no other target
     has); R_RISCV_GOT_HI20 for dynamic data; R_RISCV_TPREL_HI20,
     TPREL_LO12_I, TPREL_LO12_S and TPREL_ADD (local exec TLS, as 0571's
     test binaries needed initial exec in PIE: TLS_GOT_HI20); and, with
     C, R_RISCV_RVC_BRANCH and R_RISCV_RVC_JUMP.
   - Static mode (0243) and dynamic glibc linking with the interpreter
     /lib/ld-linux-riscv64-lp64d.so.1, PLT and GOT.
   - Branch islands: elflink's islands (elflink_islands_test) carry over
     for `jal`'s ±1 MiB across parts.
   - Debug information and CFI: DWARF register numbers per the RISC-V
     psABI (x0 to x31 are 0 to 31, f0 to f31 are 32 to 63).

5. **Atomics, the memory model and the fiber switch.**
   - Atomics use the A extension, which RV64GC always has: `amoadd.d`,
     `amoswap.d`, `amoor.d`/`amoand.d` with `.aq`/`.rl`/`.aqrl`, and
     `lr.d`/`sc.d` loops for compare-and-swap, per the RISC-V ISA
     manual's mapping of the C11 orders. Plain acquire loads and release
     stores use the fence mapping (`ld; fence r,rw` and `fence rw,w; sd`).
     arm64's `ldar`/`stlr`/`ldaxr`/`stlxr` templates map one to one.
   - Fiber switch (rvrt.xo): save and restore ra, sp, s0 to s11 and fs0
     to fs11. Reference: the LLVM port's switch asm (0571; Go's
     runtime/asm_riscv64.s gogo).
   - Async preemption (0318): the SIGURG handler reads sigcontext at 176
     (the pc, then x1 to x31), which 0571 already lands in pool.xo. The
     stub saves every caller saved integer and float register (Go's
     preempt_riscv64.s).
   - Unwinding: the s0/ra frame walk at fp-16/fp-8 that unwind.xo
     already has for LLVM riscv64.
   - System calls: `ecall`, the number in a7 (the asm-generic table, with
     0571's exceptions such as renameat2), as the LLVM port does.
   - RVWMO is weaker than TSO and close to arm64's model. The runtime's
     orders were made right on arm64, but qemu's TCG runs memory more
     strongly ordered than hardware does, so ordering bugs show only on
     real hardware (point 7).

6. **The LLVM build is the reference.**
   - Every native riscv64 golden's output must equal the LLVM riscv64
     build's, which is want_linux_riscv64.txt where 0571 added one,
     else want.txt.
   - progen differentials run native riscv64 against LLVM riscv64,
     both under qemu.
   - For speed, LLVM riscv64 at -O2 is the comparison, as LLVM is for
     arm64.
   - riscv64's own asm pattern tests (`test/codegen` `--backend=riscv64`)
     guard the templates.
   - The arm64 and amd64 output stay byte identical: TestGoldenAsm and
     the fixed point pass on every change to the shared front half,
     0254's gate.

7. **The test host.**
   - Pass/fail: qemu-user on offload host 2 (lane pass2), installed by
     scripts/riscv64-toolchain.sh under /work/riscv64 (0571; offload.md
     "linux/riscv64 under qemu-user"). This needs no new setup. The Mac
     cross-builds (native riscv64 needs no sysroot for static programs),
     and a riscv-run.sh in the shape of amd64-run.sh runs the goldens on
     host 2.
   - Instruction counts: qemu's TCG plugin `libinsn.so` counts guest
     instructions deterministically. This is the callgrind analogue for
     a codegen-only claim (native against LLVM), allowed by the
     measuring rules for codegen changes. It is not wall time.
   - Wall time and ordering: there is no riscv64 hardware on any offload
     host (offload.md: host 1 and host 2 are linux/arm64, the amd64 host
     runs FreeBSD and Windows guests). `--ab` wall claims and the
     weak-memory soak need a real machine. Options, for the owner to
     pick:
     - Rent: Scaleway Elastic Metal RV1 (EM-RV1-C4M16S128-A: T-Head
       TH1520, 4 C910 cores at 1.85 GHz, RV64GC with T-Head's own
       extensions and no standard Zba/Zbb, 16 GB, 128 GB, 100 Mb/s).
       Cost: 15.99 EUR a month or 0.042 EUR an hour, excluding VAT, with
       no setup fee (Scaleway Labs' EM-RV1 page, checked 2026-11-15; the
       price is unchanged since the 2024 launch). It is sold as an
       evaluation machine, and is the only riscv64 bare metal rented by
       the hour that we know of; AWS, GCP and Azure offer none. It suits
       RV64GC (rva20u64) timing, not the rva22u64 level.
     - Buy: a Milk-V Jupiter (SpacemiT K1 or M1: 8 cores, RVA22 with
       V 1.0; mini-ITX). Cost: about 60 USD for the 4 GB K1 and about
       115 USD for the 16 GB M1 (2024 to 2025 reports; marketplace
       listings run up to 259 USD), plus a power supply and storage. It
       joins the owner's network as a third offload host.
     - Buy: a Banana Pi BPI-F3 (the same K1 SoC as a board). Cost: about
       74 to 89 USD for 4 GB/16 GB, and up to 230 USD for 16 GB/128 GB at
       some sellers. The 4 GB model is listed end-of-life at one EU
       shop. It is slower to set up than the Jupiter (eMMC, no ITX case).
     - The K1 boards cover both levels (rva20u64 and rva22u64, Zba/Zbb
       and V 1.0), and the EM-RV1 covers rva20u64 only. For timing alone,
       a 16 GB K1/M1 board is the better fit; the EM-RV1 costs nothing up
       front and nothing to keep on the owner's network.
     - Without either, the backend lands on qemu evidence (goldens, fixed
       point, instruction counts) and the wall `--ab` waits for hardware,
       listed as an open item.

## Plan (features first, then the evidence)

1. **Encoder and ELF.**
   - enc_riscv64.xo covers RV64IMAFD, Zicsr and Zifencei, in 4-byte
     forms only, with llvm-mc goldens for every form.
   - elflink and elf handle machine 243 with the relocations above, in
     static and dynamic modes.
   - Done when a hand-written assembly hello world, assembled and linked
     by Xo, runs under qemu, and clang's object of the same text is byte
     equal.
2. **Code generation.**
   - rv.xo retarget, rvemit.xo, rvrt.xo; regalloc's riscv64 sets;
     LP64D calls; frames and CFI; fault stubs; atomics; the fiber switch;
     thread locals; preemption.
   - `--backend=riscv64` for linux/riscv64 (`--targets=linux/riscv64`
     cross-builds from any host).
   - Done when the internal/arm64 goldens pass under qemu, with a list of
     the rest.
3. **Broad.**
   - Every golden, std's tests through XO_TEST_EXEC, the corpora, the
     amd64 and arm64 identity gates, and progen differentials against
     LLVM riscv64.
   - The self build cross-built: xo built natively for linux/riscv64,
     run under qemu, builds compiler/xo twice, and s2 and s3 are
     identical (the fixed point, slow under TCG but pass/fail).
   - riscv64 becomes the default backend for linux/riscv64 in its own
     change, as 0270 did for amd64.
4. **Size and level.** The C extension (variable sizes, branch sizing
   iterated); the `XO_RISCV64=rva22u64` level with Zba and Zbb; std's
   aligned word-at-a-time forms.
5. **Evidence.**
   - The goldens, the cross-built fixed point, and qemu instruction
     counts against LLVM riscv64 (-O2) on bench/perf, recorded in
     bench/results.
   - `--ab` of wall time against LLVM on real hardware when the owner
     provides it, plus the weak-memory soak (the concurrency goldens
     looped) there.
   - __text size against LLVM riscv64.
   - A seed for linux/riscv64 when the owner wants xo itself to run
     there (0216).

## Progress

- Stage 1 (Xo Lang 1, 2026-11-15): done. enc_riscv64.xo encodes 595 forms to
  llvm-mc 23.1.3's bytes (enc_riscv64_test.xo, from scripts/rv64-enc-goldens.py). `li`
  follows LLVM's RISCVMatInt. assemble_elf_rv64 writes LLVM's object: `.riscv.attributes`,
  the ISA named on the first `$x`, `.L0 ` FDE begin symbols, and calls to a local function
  of the same section resolved. clang's object of four test sources is byte equal
  ("riscv64 objects are clang's, byte for byte"). elflink links them static and with
  glibc 2.27, and under qemu-riscv64 on host 2 they print `hello` and `hello from
  riscv64` (gotchas/riscv64-host-tools.md).

## Consequences

- linux/riscv64 builds no longer need clang or a sysroot for static
  programs, and build at native backend speed. LLVM stays as
  `--backend=llvm` and as the reference.
- compiler/arm64 grows by an estimated 5,000 to 6,000 lines (fewer than
  x86's 7,000: no two-operand rewrite), and compiler/macho by about
  1,500 (the encoder, then C compression). Every change to the shared
  front half now has a third target to keep green. The riscv64 goldens
  run on host 2 under qemu, which is slower than native, so they belong
  in the 8-hourly full tier, with a small sample per push for changes
  that touch the shared front half (as the amd64 sample does).
- Some performance work is riscv64 specific: flag-free compares,
  constants without movk, two-instruction pair accesses, and alignment.
  Until the level and vector stages land, riscv64 code will be larger
  and slower per clock than arm64's.
- Timing claims for riscv64 wait for hardware. Until then, qemu
  instruction counts are the only speed evidence, and they say nothing
  about the memory system.

## Alternatives

- **Stay LLVM only (0571's Decision 2).** This costs nothing, but every
  riscv64 build pays clang. Rejected by the owner's priority of
  2026-11-15.
- **A separate compiler/riscv64 backend with its own isel.** It could
  produce better code from the start (no arm64 shapes to undo), but it
  would duplicate isel, calls, closures, rc and the MIR passes (more
  than 15,000 lines). The retarget route landed x86 broadly first and
  replaced the costly shapes function by function. The same applies
  here, and 0254 stage 2b's split can later make the shared half
  target-neutral.
- **Emit machine code directly, without text.** 0274 kept the text
  interface (only the amd64 assembler speedup is funded), so riscv64
  follows the same text interface, with llvm-mc as its oracle.
- **The C extension or RVA22 in the baseline.** This gives smaller,
  faster code, but excludes RV64GC-only boards (C is in every shipping
  Linux-capable core, but making it the default before the encoder's
  sizing is proven risks miscompiles). C comes in stage 4, and RVA22
  stays an opt-in level, as Go's GORISCV64 does.
