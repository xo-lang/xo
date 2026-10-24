# SSA joins carry every live variable before cleanup

Status: Observed 2026-10-24

## Symptom

A pass in cleanup_func (compiler/ssa constreuse.xo) matched const_read's
shape in a hand-built test function but never fired on a real program:
DS-2's loop kept its four constant reads, and the instruction counts
did not move.

## Cause

The builder gives a join block a parameter for every variable live
there, not only the value it was made for (const_read's join is made
with one parameter, the list, and the builder adds the loop's `n` and
`i`). remove_trivial_params drops the ones every edge passes the same
value only after the passes in cleanup_func. The matcher required the
join to have exactly one parameter.

## Fix

Find the parameter by the edge argument's position (the built path's
edge passes the value at index pi; the join's params[pi] is the
result), and when rewriting a jump to that join pass the other edge
arguments as they were. A test that builds the shape by hand gives its
joins an extra variable parameter too (constreuse_test.xo read_shape).
