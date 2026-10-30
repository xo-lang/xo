# Splitting a hot runtime function's rare case into a callee can push it out of the inliner

Status: Observed (xo-lang-6, 2026-10-30, decision 0425)

Symptom: the alloc bench's tasks workload ran 1.145 times the instructions after the page block
change, mixed 1.054, and callgrind put the extra in obj.rl_free_n_slow and obj.hresize: they called
alloc.free_push and alloc.resize where the base inlined them (14 instructions a free for the call,
its frame and saves).

Cause: XO_INLINE_REPORT=1 showed free_push at 78 instructions (limit 40 for a function inlined at
every site) and resize's prefixed body at 316 with two sites (limit 500 for the copies). The rare
case (a page aligned block) had been sent out of free_push with `return free_push_aligned(h, p)`:
a call makes the function a non leaf in compiler/arm64/inline.xo inline_walk even in tail position
(shrink.xo mark_tails emits it as a branch), so every constant counts as an instruction (34 to 78).
resize's prefixed path had been moved to a function that both resize and resize_aligned called, so
its two sites put the copies over the budget. Splitting a cold path off a hot function is the usual
advice (ssa.cold_fn), and here it did the opposite.

Fix: run `XO_INLINE_REPORT=NAME xo build` (or 1 for every function) on a program that uses the
function before and after, and keep the hot function's line: small / loop / few / no with its
instructions and blocks. A hot function that must stay small takes its rare case as a test that
returns the slow answer (free_push returns false, free_mt tells page blocks apart) or as a tail call
of a cold function only when its own count stays under the limit; a body shared by two functions
has two sites, so the hot one keeps its own copy. Every `if` costs three blocks in the count (its
then, its else and the join, nested or not), so a function inlined at every site has room for three:
free_push's fourth test (counting a free when the heap counts) became an add of the COUNT word (0 or
1) without a branch. notes: the inliner's leaf rule for a cold tail
call is a follow-up (xo-lang-6 followups.md).
