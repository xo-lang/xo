# An unwind exit returns `rt.rl_empty` for each `var` List

Status: Observed (2026-10-13, decision 0211)

Symptom: an analysis of what a function hands back for its `var` List
parameters (decision 0211's summaries) found no function that returns
its own list: every one has a second `return`.

Cause: the SSA builders give a function that can fault an unwind exit
(decision 0085) that returns zero values, and the zero of a List there
is the call `rt.rl_empty` (the shared empty list), not `list.new`. The
LLVM IR shows `call ptr @xo_rl_empty()` either way.

Fix: treat `rt.rl_empty` as a new list wherever `list.new` is
(noalias.xo `is_new_list`). It is immortal and never written: an update
of it grows a new list first.
