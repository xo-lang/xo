# A debugger showed a variable's first value after it changed: a constant binding over the whole function

Status: Observed (2026-11-20, task #243; the owner's 0.51.1 evaluation, "locals in a debug build")

## Symptom

`var t = 0`, a loop adding to t, then a breakpoint: `xo debug` showed
`t = 0` where t was 10, on the native arm64 and x86-64 backends alike.

## Cause

A source variable has binding records (ssa DBind: from this line on it
holds this value). The native backends describe a record whose value is
a constant over the whole function, since no register holds a literal
(dwvars.xo, 2026-10-22). The compiler had folded the loop, the records
of t's later values (the loop header's join parameter, the `+=`) had
gone with their values, and the first record, the constant 0, was the
only one left.

## Fix

Three rules. The builder gives an immutable local no record for a join
parameter (build.xo debug_bind): its first value is its value. When a
record is dropped with its value, the variable's constant records at
other lines are dropped too (passes.xo): the variable is "optimized
out", which is true. And a backend describes a constant over the whole
function only for a variable whose records are all at one line
(dwvars.xo). cmd/xo TestDebugAssignedVarNotStale fails before.

A record at the constant's own line is the same declaration (a literal
converted where it is declared) and does not count as a later value.
