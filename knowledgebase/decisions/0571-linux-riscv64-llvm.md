# 0571: linux/riscv64 through the LLVM backend

Status: Proposed (Xo Lang 1, 2026-11-08; the owner decides). The owner
approved linux/riscv64 as a new target on 2026-11-08, through the lead,
"through the LLVM backend first, which costs least". This ADR is the
plan. Go's linux/riscv64 port (Go 1.14 and later) is the reference for
the runtime's assembly and system calls.

## Context

Spec 12.1 lists seven targets, all arm64, x86-64 or wasm32. RISC-V
servers and boards are shipping (SiFive, StarFive, Sophgo, the Milk-V
boards), Debian and Ubuntu publish riscv64 ports, and Go has run there
since 1.14.

What a new architecture touches today (main feb9f7323):

| Part | Where | riscv64 |
|---|---|---|
| Target name in the language | spec 4201: `TargetArch` is `Arm64 Amd64 Wasm32`; std matches on it exhaustively | add `Riscv64`: a spec change; the compiler then lists every `match target.arch` that lacks the arm |
| Native code generation | compiler/arm64 (arm64 and x86-64), compiler/wasm | none: not ported (Decision 2) |
| LLVM code generation | compiler/llvm: the target triple, the `svc #0` system call asm (call.xo 1419), the context switch `switch_asm` (sched.xo), the async preemption stub (apreempt.xo), the raw clone and exit stubs (gen.xo 1379, 1402) | a riscv64 form of each (about 36 arch sites in call.xo, gen.xo and sched.xo) |
| System call numbers | std/native/llrt linux.xo `nr(arm64, amd64)` | riscv64 Linux uses the asm-generic table, as arm64 does: `nr` returns the arm64 number |
| Runtime per arch code | about 29 llrt files match on `TargetArch` | mostly SIMD and crypto fast paths (portable fallback), the page size (4096), the vDSO symbol (`__vdso_clock_gettime`), and the unwinder and fault frames (unwind.xo reads x29 and x30) |
| Linking | the LLVM backend links with clang and lld; a linux cross build takes a glibc sysroot (clang.xo llvm_cross_flags) | `riscv64-linux-gnu`, a Debian riscv64 sysroot on the Mac and the offload hosts |

## Decision (proposed)

1. **Target name and triple.** `linux/riscv64` with
   `TargetArch.Riscv64`, built only with `--backend=llvm` as
   `riscv64-unknown-linux-gnu`, `-march=rv64gc -mabi=lp64d` (RV64GC:
   integer, multiply, atomics, single and double float, compressed;
   what Debian, Ubuntu and Go's default `GORISCV64=rva20u64` assume).
   It is not part of `--targets=all` until stage 3 below passes;
   naming it without `--backend=llvm` is an error that says to add it.
2. **No native backend at first.** compiler/arm64's instruction
   selection, register allocator and encoder serve arm64 and x86-64
   only. A native riscv64 backend is a later ADR, decided with numbers
   from the LLVM port (build time and code size against linux/arm64's
   native build). Until then LLVM is riscv64's only backend.
3. **Runtime glue, each with Go's file as the reference:**
   - System calls: `ecall` with the number in a7, arguments in a0 to
     a5, the result in a0 (a negative errno), clobbering memory (Go's
     runtime/sys_linux_riscv64.s). linux.xo's `nr` returns the
     asm-generic (arm64) number for `Riscv64`.
   - Thread pointer: the `tp` register (x4) holds the TLS base;
     `rt.tlocal_get` and `set` keep using LLVM's thread local storage, as
     on arm64.
   - Context switch (fibers): save and restore ra, sp, s0 to s11 and
     fs0 to fs11 (the lp64d callee saved set); the switch is naked asm
     as on arm64 (sched.xo switch_asm). Go's runtime/asm_riscv64.s
     gogo and mcall are the reference.
   - Async preemption (decision 0318): the signal handler redirects the
     pc to a stub that saves every caller saved integer and float
     register (Go's runtime/preempt_riscv64.s).
   - Atomics: LLVM's `atomicrmw` and `cmpxchg` lower to the A
     extension's `amoadd.d`, `amoswap.d` and `lr.d`/`sc.d` loops with the
     `.aq` and `.rl` bits; no outline helpers are needed (RV64GC always
     has A).
4. **Memory model.** RISC-V's RVWMO is weaker than x86-TSO and close
   to arm64's. Every runtime atomic already states its order (relaxed,
   acquire, release, acq_rel, seq_cst), and LLVM maps them by the RISC-V
   ISA manual's mapping (fences and `.aq` and `.rl`). The runtime's
   plain reads of shared data must already be correct on arm64; riscv64
   adds no new class of race, but it is a second weak-memory platform
   that may catch ordering bugs arm64 happens to hide. Decision 0058's
   shared reference counts carry over unchanged.
5. **Unwinding and faults.** The fault traceback and XO_PROF walk
   frames with the unwind tables where they can (LLVM emits DWARF CFI
   in `.eh_frame` for riscv64); unwind.xo's frame pointer walk reads
   x29 and x30, while riscv64's frame pointer is s0 (x8) and its return
   address ra (x1), with the saved pair just below the frame pointer
   (`fp - 16`, `fp - 8`), unlike arm64's. Stage 2 adds the riscv64
   step; until then a fault prints the faulting function only.
6. **Testing.** Pass/fail runs under qemu-user (`qemu-riscv64-static`,
   through binfmt or named on the command line) on offload host 2 (lane
   pass2), as wasm runs under Node: the LLVM goldens, xo test of std,
   and the corpora. qemu's TCG runs memory accesses more strongly
   ordered than the hardware does, so concurrency tests prove less
   there: a run on real hardware (a riscv64 board or a cloud machine)
   joins the full tier when one is available. Timing under qemu is not
   a measurement.

## Plan

1. **Stage 1: hello world and the goldens qemu passes.** `TargetArch`
   gains `Riscv64` (spec 12.1 and 4201, std's matches); the triple and
   flags; the system call asm, clone and exit stubs; the context switch;
   no async preemption yet (loops poll, as before decision 0318); a
   Debian riscv64 sysroot on host 2 and the Mac. Done when hello world
   and the LLVM goldens that need no network run under qemu, with a
   list of the rest and why.
2. **Stage 2: faults, profiles, preemption.** The riscv64 frame step
   in unwind.xo and prof.xo, the async preemption stub, the fault
   goldens and XO_PROF.
3. **Stage 3: in `all`.** Every LLVM golden and std's tests under qemu
   (the ones that need real hardware listed), riscv64 added to
   `--targets=all` with `--backend=llvm`, and a seed for it if the
   owner wants xo itself to run there (xo built by LLVM for riscv64).
4. **Later: a native backend**, in its own ADR (Decision 2).

## Progress

- 2026-11-08, stage 1 and stage 2's frame step land together:
  hello world runs under qemu-user on host 2 (a 1.3 MB static ELF);
  the toolchain is scripts/riscv64-toolchain.sh (offload.md). Of the
  523 LLVM goldens that need no network, all 523 pass under qemu (each
  batch at the branch's commit of its day; zip_entry_limit takes over
  two minutes there). On the way: the
  22 caught-fault goldens pass with the riscv64 unwinder (unwind.xo's
  register offsets, sched.xo's save and resume); four fs goldens needed
  renameat2 (gotchas/riscv64-syscall-table-not-arm64.md); f32_bits keeps
  riscv64's canonical NaNs from arithmetic in want_linux_riscv64.txt,
  while conversions keep a NaN's payload in integers (num_wrapping, and
  the new f32_nan_list; decision 0361's addendum). The build fixes are
  in gotchas/riscv64-llvm-default-rv64i.md.
- 2026-11-08, stage 3 started: `xo test` runs std's tests under qemu
  through XO_TEST_EXEC (toolchain.md); test binaries (PIE, no LTO)
  needed the thread local table as initial exec
  (gotchas/riscv64-tls-get-addr-in-tests.md). Std's 60 test modules
  (35 under std/native, 25 under llrt) pass, about 4 minutes in three
  batches. `--targets=all` stays as it is: the LLVM backend builds one
  target per build, the native backends' `all` leaves out targets they
  do not build, and only `--backend=go` would add linux/riscv64, where
  the Go backend's runtime is untested.

## Consequences

- One more target to keep green, on a backend that is not the default:
  clang's build times apply to every riscv64 build.
- `TargetArch` grows: user code that matches on `target.arch`
  exhaustively must add an arm. Spec 12 is [P1]; the change is listed
  in "Changes in 0.N".
- std's SIMD fast paths (bytes_skip32, the crypto routines) take their
  portable forms on riscv64 until RVV (the vector extension) versions
  are written, so those paths run slower per clock than on arm64.
- qemu-user is slow (TCG): a full golden run takes longer than on
  arm64; it runs on host 2 in the pass/fail lane.

## Alternatives

- **A native riscv64 backend first.** The most work (instruction
  selection, a third register allocator target, an encoder, the
  linker's relocations) for a target with no users yet; the LLVM path
  gives a working port to measure first.
- **The Go backend** (`--backend=go`, Go's linux/riscv64): it works today
  for the programs the Go backend builds, but it is the reference
  backend only and does not exercise Xo's runtime; it stays available.
- **RV64GCV (with vectors) as the baseline.** Many boards shipping in
  2026 lack RVV 1.0 (some ship the 0.7.1 draft); RV64GC is what
  distributions target. RVV versions of the fast paths can be chosen at
  run time later, as the x86-64 ones are by CPU features.
