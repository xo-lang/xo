# 0619: Arm's DIT bit around std's constant time code

Status: Proposed (2026-11-12, Xo Lang 3; the lead approved the plan the
same day, with the scheduler, every-build and amd64 points below)

## Context

Decision 0222 keeps std's constant time code free of data dependent
branches and selects (rt.ct_barrier), and decision 0616's review checks it
with dudect. The 0616 RSA review's dudect of rsa.pkcs1_unpad and
rsa.oaep_unpad alone (std/native/llrt/rsa_test.xo) passes natively (max
|t| 2.3) and on LLVM at -O2 (1.7), but fails in `xo test`'s LLVM -O0
build on Apple silicon (|t| 81 between padding classes, 5.7 between two
sets of one class), whose assembly has no data dependent branch or select:
-O0 keeps the loop's values on the stack, and the time of those loads and
stores followed the data (gotchas/dudect-o0-llvm.md). Explicit masking
cannot fix it: the code already is masks. Users ship -O0 and debug builds,
and constant time must not depend on the optimizer.

Arm's FEAT_DIT (Armv8.4, PSTATE.DIT, EL0 writable) makes the timing of a
listed set of instructions independent of their data, and Apple documents
that DIT also turns off its data dependent predictors on M3 and later. Go
1.24 sets it in crypto/subtle.WithDataIndependentTiming and around its
crypto. A C replica of pkcs1_unpad on this Mac (3x10^5 measurements):

| build | DIT off | DIT on |
|---|---|---|
| -O0 | \|t\| 23.7 | \|t\| 1.08 |
| -O2 | \|t\| 1.84 | \|t\| 0.80 |

DIT cost in that loop: +25% at -O0, +11% at -O2.

## Decision

1. **The instruction.** std/native/llrt/cpu.xo gains `dit_set(on: Int)
   -> Int` (decision 0231's pattern): it sets PSTATE.DIT to on and returns
   the previous value. Its portable body returns 0 and does nothing, so a
   wasm module, amd64, riscv64 and an older xo (a seed building xo) run
   the body. The arm64 emitter lowers it to `mrs xd, s3_3_c4_c2_5` and
   `msr s3_3_c4_c2_5, xs` (the generic names every assembler takes;
   compiler/macho's encoder gains 0xd53b42a0 and 0xd51b42a0), LLVM to the
   same inline asm with sideeffect, and neither is moved or merged
   (IF_KEEP, as rt.ticks).
2. **Only on a CPU with DIT.** cpu.hw() gains HW_DIT: every Apple arm64
   CPU on darwin, AT_HWCAP's DIT bit (1 << 24) on Linux. On a CPU without
   FEAT_DIT (Graviton2, Neoverse N1, Armv8.2) the instruction is
   undefined, so ct.dit_on() calls dit_set only with HW_DIT and returns 0
   otherwise. The check reads a cached word, a public branch.
3. **Where.** std/native/llrt/ct.xo: `dit_on() -> Int` (the old value)
   and `dit_off(old: Int)` (restores it, so sections nest). std's secret
   dependent entry points wrap their whole operation, not each helper:
   RSA decrypt and sign (the private exponentiation and the unpad), the
   ECDH and ECDSA scalar multiplications, Ed25519 signing, X25519,
   ML-KEM decapsulation, the software AES and GHASH paths, and
   ct.bytes_eq (HMAC and tag compares). Every arm64 build has them: native
   and LLVM, at every opt level. std/crypto exposes no API for it (a user
   who writes constant time code calls crypto.constant_time_eq).
4. **A section does not yield.** PSTATE.DIT is per thread: the kernel
   saves it in a thread switch, but a fiber switch does not, so a fiber
   that yielded in a section would leave DIT set on the worker for other
   fibers and run on without it after resuming on another worker. So:
   - the code between dit_on and dit_off makes no call that can park or
     yield (no I/O, channel, lock, sleep or spawn): pure computation over
     bytes it was given. A test build checks it: sched's fiber_yield
     faults when DIT is set (`mrs` in a test build only);
   - asynchronous preemption (decision 0318) declines while the
     interrupted code has DIT set: pool.async_signal reads PSTATE from the
     ucontext (darwin arm_thread_state64.cpsr, Linux sigcontext.pstate;
     bit 24) and returns, as it does for a runtime lock; the monitor asks
     again at its next scan. A section is short (an RSA-2048 private
     operation, the longest, is about 1.5 ms), so a fiber holding DIT is
     preempted at most that late;
   - the poll path (pool.preempt) declines the same way, reading DIT
     with `mrs`.
   A thread switch by the kernel, and the signal frame of any other
   signal, save and restore PSTATE, so DIT survives them.
5. **amd64.** x86's equivalent, DOITM (Intel's data operand independent
   timing mode, MSR 0x48 bit 0), is set only by the kernel (Linux has no
   user interface for it yet), and AMD has no such mode. dit_set is the
   no-op body there. spec/std.md's crypto section says so: on amd64 the
   constant time property is the code's alone. The fallback: if a dudect
   row on the amd64 host shows the -O0 build leaking, std's constant time
   llrt modules (ct, rsa, bigmod, the curve fields) build at -O2 in every
   LLVM build (a per-module opt level in split.xo), so their values stay
   in registers.
6. **Evidence (decision 0616).** rsa_test.xo's dudect runs on native,
   LLVM -O0 (the test default) and LLVM -O2, on the Mac and the linux/arm64
   host, and on the amd64 host; the A/A pairs stay. `xo bench --ab` on
   linux/arm64 host 1 measures DIT's cost on the wrapped paths (RSA
   decrypt and sign, ECDH P-256, X25519, Ed25519 sign, ML-KEM decaps,
   AES-GCM software path): reported in this ADR before it is Accepted.

## Consequences

- Constant time on arm64 no longer depends on the opt level or the
  backend's register allocation for the listed data processing
  instructions; a memory access's address is still the code's job
  (DIT does not hide a secret index).
- Each wrapped call pays two `msr` and two `mrs`, plus DIT's slowdown of
  the instructions in between (+11% in the C replica at -O2; the --ab
  rows decide whether a path keeps it).
- A fiber in a section is not preempted until it leaves it.
- Seeds: the body is a no-op, so an older xo builds a std that calls
  dit_set (it just does not set DIT); no seed cut is needed.
- Linux arm64 without FEAT_DIT and amd64 get no DIT; their constant time
  rests on the code (and, if needed, the -O2 module fallback).

## Alternatives

- **Per-module -O2 only** (point 5's fallback for every target): fixes
  this -O0 effect but not the CPU's own data dependent timing (Apple's
  M3 predictors), which DIT controls and no code layout does.
- **DIT set for the whole process** (at start): simplest, no scheduler
  rule, but every instruction of every program pays DIT's cost.
- **Save and restore DIT in every fiber switch**: makes yields in a
  section safe, but adds an `mrs`/`msr` to every switch of every program
  for a case std's sections never need.
- **Explicit masking** (the lead's first guess): the code is already
  masks with no branch; the leak was in the loads and stores.
