# A std store build's loops were never preempted on arm64

Status: Observed (2026-11-17, task #306).

## Symptom

TestPreempt/arm64 and TestAsyncPreempt/arm64 failed on the Mac at every
commit, b39f61874 (the bisect's good end) included: preempt_regs, a spin
loop beside a sleeping fiber, saw 0 preemptions. The same source built
whole program (no std store, as under heavy.sh after a store miss with
XO_SEPCOMP_BG=0) saw about 77. A bisect looks like it finds nothing: the
shared cache's store is what fails, not a commit.

## Cause

async_pc_ok (std/native/llrt/pool.xo) declines a pc that is in no table.
A whole program build has one table per file spanning all its text; a
std store build (decision 0388) has one table per function, and
compiler/arm64 apreempt.xo unsafe_fn_tables made one only for a function
with an unsafe range, ending at its last range. A loop in the program's
code, or any std function without a range, was in no table, so the
SIGURG handler declined every signal there (lldb: the handler ran 13,964
times and never passed the pc check).

## Fix

push_fn_text ends each function's text with a `_fe` label, and
unsafe_fn_tables gives every function a table to it (no ranges for a
function with none). compiler/arm64/unsafefn_test.xo checks the tables;
TestPreempt/arm64 checks the preemptions with the store present.

## Avoid

A preemption test passing under heavy.sh may have run without the std
store: build it first (`xo dev stdcache --conc=1`) when the change is in
the store path.
