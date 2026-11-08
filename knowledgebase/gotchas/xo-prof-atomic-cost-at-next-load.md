# XO_PROF charges an atomic's stall to the load after it

Status: Observed (Xo Lang 1, 2026-11-08)

## Symptom

XO_PROF put 9 to 13% of bench/perf/metrics counter's CPU in
ctx.unwinding (linux/arm64, 2026-11-07 and 2026-11-08), a 9 instruction
function that reads the thread's context from TLS: every sample on one
pc, the `cbnz` after its first load (`ldr x9, [x9]`). Inlining the check
(b20d756cf, 2026-11-07) changed nothing in an `--ab` (counter 0.998).

## Cause

Not the profiler's naming or unwinding. The pc is an interrupted
instruction's own (prof.xo frame_pc adds 1, dwline looks up pc - 1),
the outline atomics have symbols (`__aarch64_ldadd8_acq_rel` is named
when a sample lands in it), and a frameless helper's caller comes from
the link register (unwind.xo walk_regs). ctx.unwinding runs after
nearly every call (111 call sites in the bench's main), and the call
before it often releases a reference with an acquire-release atomic. A
load after such an atomic waits for it, and a timer interrupt that
arrives during the wait is taken at the instruction after that load.

Repro (2026-11-08, host 2, ed685dd2f): a loop calling a small function
`probe`, with or without `Atomic[Int].add(1)` before each call
(~/xo-lead/notes/lang1/skidrepro). plain 0.20 s, atom 0.36 s. The
atomic's 0.16 s is charged correctly as a whole
(`__aarch64_ldadd8_acq_rel` 99 ms, atomic.add 59 ms). Inside probe, the
instruction after its first load takes 82.6% of probe's samples with the
atomic, against 31.8% without. `perf record -e cpu-clock` gives the same
split as XO_PROF to the percent. Host 2 is a VM with no hardware PMU
(only software events, no arm_spe), so nothing there samples more
precisely than a timer.

## How to read and fix

A flat hot spot on the instruction after the first load of a small
function that runs right after an atomic or a reference release is
usually that atomic's wait. Check the caller's instructions before the
call (`go tool pprof -peek NAME -addresses`, objdump). The remedy is
fewer or weaker atomics (unshared objects skip them), not a change to
the charged function. Precise attribution needs ARM SPE
(`perf record -e arm_spe//`) on bare metal, or x86 PEBS.
