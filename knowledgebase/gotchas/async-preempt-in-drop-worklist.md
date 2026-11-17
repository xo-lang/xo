# A fiber preempted inside the drop worklist held every drop of its worker

Status: Observed (2026-11-17, xo-lang-1, task #213)

## Symptom

std/bufio's stress (64 tasks, 8 to 30 minutes, Linux arm64) grew from about 17 MB to 40 MB on the native
arm64 backend and failed its memory check, while the LLVM build of the same program stayed at 11 MB. Live
blocks sampled every minute looked normal (about 1,000), but the peak each minute reached 5,000 to 35,000
(LLVM: about 1,150), and the free lists then kept the spike's blocks (20 MB counted free). XO_NOASYNC=1
(no SIGURG) and 8 tasks instead of 64 both made it go away; whole function unsafe ranges did not.

## Cause

obj.xo rc_defer runs a thread's drop worklist with DEFER_BUSY set on its heap record and calls each
drop it queues: a closure environment's or a box's drop, which is the program's code. The native
backends' unsafe ranges (compiler/arm64 apreempt.xo) cover the runtime's text and the runtime code inlined
into the program, not the program's own drop functions, so SIGURG could preempt a fiber there. A fiber
preempted by SIGURG is pinned to its worker and runs again only when the worker has nothing else (pool.xo
pin_push, take_local), and while it waited DEFER_BUSY stayed set: every other fiber on that worker queued
its drops instead of running them, so whole iterations' temporaries stayed live until the pinned fiber
resumed and drained the queue. LLVM builds were never affected: compiler/llvm apreempt.xo holds F_NOPRE
in every runtime function that reaches the deferred release queue.

## Fix

pool.xo async_signal (and the Windows park, async_park) declines while the fiber holds the thread's drop
worklist: pool.async_held is W_LOCKS, F_NOPRE or obj.defer_busy (the record's DEFER_BUSY). On the stress:
11.6 to 12.1 MB, peak live about 1,300, about 170 declines a minute of 25,000 preemptions. The golden
async_preempt_defer asks async_held from a drop the worklist runs (internal.preempt_held_in_drop): 0
before the fix on arm64. A timing test (tasks dropping closures beside tasks checking the flag) never hit
the window in a few seconds: about 0.7% of the stress's preemptions landed in a drop. A runtime flag that
program code can run under is a preemption point to check on both backends: the native one by PC, LLVM by
F_NOPRE.
