# An index whose own evaluation grows the list reads the old list

Status: Observed (2026-10-22, Xo Lang 2, the List.insert lowering).

## Symptom

An xo built with compiler/ssa listmove.xo faulted while compiling any
program that called `List.insert`:
`fault: index 1 out of range for List of length 1 at ssa/listmove.xo:48:14`.
The line was `let slow = b.m.funcs[b.declare(impl, [ect])].name`.

## Cause

Evaluation goes left to right (decision 0326): `b.m.funcs` is read
first, then the index `b.declare(...)`, which appends a new function to
`b.m.funcs` and returns its index. The list that was already read is the
value from before the append, one element short. Lists are values, so
the append does not reach the copy that was read.

## Fix

Evaluate the call that grows the list first, then index:

    let callee = b.declare(impl, [ect])
    let slow = b.m.funcs[callee].name

call_as_via already does this. The same applies to any `xs[f()]` where
`f` pushes onto `xs`, through a `var` receiver or a field of one.
