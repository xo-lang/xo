# Xo scheduler code reads the pool's record before the pool starts

Status: Observed (2026-10-13, decision 0212 stage 2)

Symptom: with the parks in Xo (std/native/llrt/pool.xo park_wait), every
task golden passed but clock_timers died with SIGSEGV (exit 139) at its
first `clock.sleep`, on both native backends; the C path
(`XO_LLRT_C=sched`) passed.

Cause: park_wait finds the deadlock counts through a word of the pool's
record (xo_rt.c `pool.dlk`), which pool_init set. A program that only
sleeps never starts the pool (a root thread helps only once the pool
exists), so the word was still 0 and the first `slot_add` on the counts
wrote to address 8. The C read the static `dlk` directly and never
depended on the order.

Fix: the record's addresses that do not depend on configuration are
static initializers (`pool = {.once = ..., .dlk = &dlk}`), not
assignments in pool_init. When Xo code reaches C state through a
pointer in a C record, check every path that can run before the code
that fills the record (a root thread's first wait, a signal, a timer).
