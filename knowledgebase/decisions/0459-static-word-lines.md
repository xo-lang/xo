# 0459: Runtime tables 128 byte aligned, and XO_DATA_SHIFT to check layout sensitivity

Status: Proposed (Xo Lang 2, 2026-11-02; the 0.36 /alloc blocker's runtime side, assigned by
the lead; for the owner)

## Context

Main's /alloc (bench/perf/jitter) on the first linux/arm64 offload host (Neoverse) ran at
0.79 to 0.81 of v0.35.0's req/s, CPU a request 1.27 (Xo Lang 4, 2026-11-02). Linger plus
"narrow B" (slot_group) cost 25% more CPU a request than linger alone, though only
http.respond's code differed, and xo_static, rt.static_word's table of runtime globals
(decision 0207), sat 64 bytes lower (72 bytes less .text). Neoverse's
spatial prefetcher pairs 64 byte lines, so the hypothesis was false sharing at 128 bytes in
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

## Consequences

- .data grows by up to 2 x 120 bytes of alignment.
- A release comparison (scripts/release-vs-go-http.sh) or a regression chase can add a
  shift 64 build to tell data placement from code.
- The 0.36 gap's levers are elsewhere: the Response ws field (-9.4% CPU a request,
  layout independent) and code placement (XO_ARM64_PAD sweep).

## Alternatives

- Padding every static word to its own line: 32 KiB of .data for 256 words; not needed by
  the measurements.
- A .text shift knob in the linker: decision 0075's XO_ARM64_PAD (nops before every
  function) already tests code placement.
