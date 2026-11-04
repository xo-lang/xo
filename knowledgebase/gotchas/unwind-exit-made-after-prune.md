# A function's unwind exit can be made after prune_unwind

Status: Observed (Xo Lang 2, 2026-11-03, decision 0482).

## Symptom

A pass placed at the end of `prune_unwind` (compiler/ssa passes.xo) to
rewrite each `Mutex.with` closure's unwind exit changed nothing for
closures whose only fault points are their own operations (an index, a
checked `%`): the closure still handed back the value it was given, and
a 1,000,000 element list was copied on every `with` (a 20 line repro:
9.3 s on darwin/arm64, 0.00 s once fixed). Its `Func.unwind_exit` was
-1 when the pass ran.

## Cause

`finish_fn` builds the unwind exit at build time only for a function
with a check (a call that can fault or be canceled). A function whose
fault points are its own operations gets its exit later, in
`add_landings` (passes.xo `unwind_exit()`), which runs after
`prune_unwind` and calls `finish_fn` then.

## Fix

Decide in `finish_fn` too: an exit made after `prune_unwind` reads
`Builder.unwind_may` (set by `prune_unwind`, empty before), and the
post-prune pass covers the exits made at build time. Check a change to
an unwind exit with `xo dev dump ssa --stage=rc --fn=NAME` on a closure
with and without a call that can fault.
