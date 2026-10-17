# The native backends read a `var` receiver before the call's arguments

Status: Observed (2026-10-10, compiler/check stage 2 built with arm64 and
LLVM); fixed by decision 0142.

Symptom: the arm64 and LLVM builds of compiler/check faulted on almost
every run with `index 692 out of range for List of length 692` in
`implements` or `resolve`; the Go backend build matched the Go checker
on every run. Reduced: `t.get(t.add(7))`, where `add` pushes onto
`t.items` and `get` reads it, faulted natively with index 0 of a list of
length 0.

Cause: internal/ssa read the `var` receiver's value when it lowered the
call, before the arguments, ran the method on that stale value, and
stored it back after the call over the arguments' changes. The builtin
`List.push` and `List.set` lowering read the list before their
arguments the same way (`xs.push(f())` where f pushes onto `xs`).

Fix: the receiver's place is built first, its value read after every
argument (`callAsVia`, and the List, Buf, and `Map.update` lowerings).
Golden: `internal/gen/testdata/golden/var_recv_after_args`. Decision 0140
was the same mistake for assignments: any lowering that reads a place
and later writes it back must read it after evaluating everything else
in the statement that can change it.
