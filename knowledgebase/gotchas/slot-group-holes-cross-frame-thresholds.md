# slot_group's holes can push a frame over the 504 byte prologue

Status: Observed (2026-11-02, regression B)

## Symptom

The /alloc HTTP benchmark (bench/perf/jitter) lost 22% of its requests a
second at 32 connections and 22% at 256 after slot_group (3b0ccbb29),
although the pass made xo's code 2.4% smaller and its own --ab runs
(self, alloc, strings) were neutral or better.

## Cause

slot_group gives a run of word copies consecutive spill slots. A run's
base is chosen when its first member spills, and members that later take
a register leave their slot empty. Those holes grow the frame:
http.respond's went from 368 to 576 bytes, past the 504 bytes the
prologue's single `stp x29, x30, [sp, #-N]!` reaches. Others on the
request path grew too (serve_h1's closures). The cost showed on the
HTTP/1.1 park and resume path, which runs these functions per request.

Store-to-load forwarding (a 16 byte q copy over two 8 byte stores) was
the first suspect and was disproven: a guard that kept such copies word
by word recovered nothing (0.778 against main's 0.776 at c32).

## Fix

regalloc.xo allocates again without the runs when their holes alone take
the frame over 504 bytes or 4 KiB (FRAME_PAIR_MAX, PROBE_FRAME). Count
holes, not just frame size: a frame that crosses anyway keeps the runs.
Measure frame changes on the request path with the HTTP benchmark, not
only with self and alloc --ab runs.
