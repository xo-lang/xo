# 0459: Runtime tables 128 byte aligned, and XO_DATA_SHIFT to check layout sensitivity

Status: Accepted (owner, 2026-11-02, including items 4 and 5: the 16 byte aligned
callee-saved area, 7afa6e957, and the XO_STACK_SHIFT check knob; proposed by Xo Lang 2,
2026-11-02, the 0.36 /alloc blocker's runtime side, assigned by the lead)

## Context

Main's /alloc (bench/perf/jitter) on the first linux/arm64 offload host (a Linux VM on an M4 Pro, knowledgebase/infrastructure/offload.md) ran at
0.79 to 0.81 of v0.35.0's req/s, CPU a request 1.27 (Xo Lang 4, 2026-11-02). Linger plus
"narrow B" (slot_group) cost 25% more CPU a request than linger alone, though only
http.respond's code differed, and xo_static, rt.static_word's table of runtime globals
(decision 0207), sat 64 bytes lower (72 bytes less .text). The host's
Apple cores have 128 byte cache lines (the hypothesis first named a Neoverse prefetcher that
pairs 64 byte lines; the host is not a Neoverse, corrected 2026-11-02), so the hypothesis was false sharing at 128 bytes in
that table, whose phase against the lines moved with every change to .text or .data
before it (`.p2align 3`). Xo Lang 3 listed its hot words (read on every effect or poll:
twflags 32-33, W_PREEMPT 76, the Date cache 69-74; written from several cores: heapos'
central and cache locks 22, 24, chunk count 23, poll's timer heap 40-42).

## Finding: refuted on main

One host 1 job (2026-11-02, 3 rounds, /alloc closed loop, servers on CPUs 0-5, the loadgen
on 6-11, ~/xo-lead/notes/xo-lang-2/alloc-shift.log), every server built on main a144a3115
and linked twice, with XO_DATA_SHIFT 0 and 64 (below). Median CPU µs a request (ratio to
the first row), req/s; every row at both shifts, with ranges, the addresses and the method:
bench/results/alloc-shift-2026-11-02.md:

| server | shift | c256 | c32 |
|---|---|---|---|
| main (knob only) | 0 | 44.7 (1.000), 134k | 44.9 (1.000), 133k |
| main (knob only) | 64 | 44.9 (1.006), 133k | 45.4 (1.011), 132k |
| xo_static aligned to 128 (Xo Lang 1's) | 0 / 64 | 0.995 / 0.997 | 0.995 / 1.004 |
| both tables aligned, the written words on lines of their own | 0 / 64 | 1.000 / 0.996 | 1.002 / 1.006 |
| that, and the pool record's hot words 128 bytes apart | 0 / 64 | 0.992 / 0.998 | 1.000 / 1.008 |
| that (without the pool split), and Response without its ws field (Lang 4's ec2398ed8) | 0 / 64 | 0.906 / 0.907, 148k | 0.908 / 0.908, 146k |

At shift 64 main's xo_static moved from 0x4fc388 to 0x4fc3c8, a different 128 byte phase
(nm, in the log), with the code byte for byte the same: CPU a request moved 0.6% and 1.1%.
No padding or alignment moved it by 1%. The 25% follows code, not data: Xo Lang 1's probes,
which grow .text, shift every function after them as well (Lang 4's l4pts11: both as slow
as linger+B); decision 0075's XO_ARM64_PAD sweep tests code placement next (Lang 1).

## Decision

1. xo_static and xo_sys_words are 128 byte aligned on every native backend (arm64 and
   x86-64 `.p2align 7`, compiler/arm64 gen.xo end_data; LLVM `align 128`): which of
   their words share a line no longer changes with unrelated .text and .data. No speed
   change was measured (table above); this removes one variable from later layout chases
   at a cost of up to 240 bytes of .data.
2. `XO_DATA_SHIFT=N` makes Xo's ELF linker put N bytes of zeros (a multiple of 8, at most
   4096) before .data's first input section, so one program can be measured with its .data
   0, 64 and 128 bytes on without touching its code
   (~/xo-lead/notes/xo-lang-2/bin/alloc-shift.sh REVS SHIFTS NAME ROUNDS). A 128 byte
   aligned input rounds the shift up to its alignment, so with (1) the two tables do not
   move; what can is what is not aligned. Linux links only (darwin's __DATA starts its own
   page). When a speed change has no code cause, measure it at shift 0 and 64 before
   bisecting.
3. Not done: moving the written words to lines of their own (heapos 128-191, poll
   192-223) and splitting the pool record; both measured within 1% and stay on the
   branches l2-pad and l2-poolpad0 for reference.
4. The 0.36 gap's cause (2026-11-02, bench/results/stack-phase-2026-11-02.md): one 4 KiB
   phase of the fiber stack. linger plus narrow B made http.respond's frame 208 bytes
   smaller, so every frame of the handler's JSON encoder sat 208 bytes higher; at that
   phase (a comb of bad offsets inside about 128 bytes of the 4096) /alloc cost 36 to 40%
   more CPU a request on the first linux/arm64 host (a Linux VM on an M4 Pro, 4 KiB guest
   pages). Not code placement, .data, allocations, faults, the kernel, huge pages or
   decision 0395's stack Str buffers; darwin on the same M4 has no bad phase. On a bare
   metal Graviton 4 the same phases cost under 1% and st_align_lat (stores paying
   misaligned latency) is about 30x at exactly them, sampled in the prologues of hot leaf
   callees (num.json_layout, num.dbox, obj.rs_append): compiler/arm64 emit.xo stored the
   callee saved registers in pairs (stp) from spill_base + 8 * slots, often 8 mod 16, and a
   16 byte pair at 8 mod 16 crosses a 4 KiB page at one sp phase in 256.
   `XO_STACK_SHIFT=N` (runtime, read at start: every fiber's first frame N bytes lower, a
   multiple of 16, at most 64 KiB) is the check knob beside XO_DATA_SHIFT: a speed change
   with no code cause is measured at a few shifts (0, 16, 208, 1024, 2048) before bisecting.
5. The callee saved registers start at a 16 byte aligned offset in both frame shapes, and
   saves() pairs two only at a 16 byte aligned offset, so no stp of the native backend's
   prologues straddles a page (a server built with it: 2,550 sp pairs, none at 8 mod 16,
   against 674 before). On host 1, linger + B with it runs 35.1 to 35.3 us a request at c32
   at every former bad shift (0, 48, 64, 96, 4080) and at 16, against 47.7 without it, and
   linger at its bad shift 3888 34.6 against 47.8; v0.35.0 35.8. One frame grows by 8 bytes
   at most; xo's __text +216 bytes.

## Consequences

- .data grows by up to 2 x 120 bytes of alignment.
- A release comparison (scripts/release-vs-go-http.sh) or a regression chase can add a
  shift 64 build to tell data placement from code.
- The 0.36 gap's levers are elsewhere: the Response ws field (-9.4% CPU a request,
  layout independent) and code placement (XO_ARM64_PAD sweep).
- (4, 5) A frame size change upstream no longer moves /alloc's CPU by a third on Linux
  arm64 VMs: main itself sat on a bad phase on 2026-11-02 (43.7 us at c32, 35.5 with the
  stack spread under Alternatives). Other stores that can cross a page remain (an unaligned byte
  store into a buffer, a q register pair at 16 mod 32); the Graviton counters showed only
  the register pairs following the stack phase.

## Alternatives

- Spreading fibers' first frames over 64 phases of a 4 KiB page (a per-worker count;
  Xo Lang 2's experiment commit e9eaf8dac, not on main): it took main from 43.7 to 35.5 us
  and linger + B from 47.9 to 35.1, but a fiber landing on a bad phase still paid in full,
  and with no pair crossing a page it has nothing left to bound.
- Huge pages for fiber stacks: measured (XO_STACK_THP experiment), the gap stays.

- Padding every static word to its own line: 32 KiB of .data for 256 words; not needed by
  the measurements.
- A .text shift knob in the linker: decision 0075's XO_ARM64_PAD (nops before every
  function) already tests code placement.
