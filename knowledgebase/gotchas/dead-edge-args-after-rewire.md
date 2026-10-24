# A removed path's edges must pass nothing

Status: Observed 2026-10-24

## Symptom

xo built with ssa constreuse.xo's first version faulted building
compiler/xo (the fixed point's s1): `index -1 out of range` in
ssa.succ_edges (uniqvar.xo), from Rc.apply_uniq.

## Cause

A pass in cleanup_func that rewires a block (a read's start block now
jumps straight to its join) makes the old path unreachable. prune_func
drops those blocks but keeps their edges in f.edges, from = -1 after
remove_blocks. uniq_uses reads the arguments of every edge in f.edges,
so a dead edge still passing a live value gives a use at block -1.

## Fix

A pass that makes blocks unreachable clears the arguments of their
edges (constreuse.xo reachable()), as the builder never leaves a dead
edge with arguments. A test checks that no unreachable block's edge
passes a value.
