# Generic List methods with a copy call do not inline into generated code

Status: Observed (2026-10-08, bench/perf/llvm sieve, std/rt/list.go)

Symptom: after List writes became in place, the sieve still ran at 3.6x
of Go; a profile showed `(*List[go.shape.bool]).SetAt` and `M_push` as
separate functions with most of the time.

Cause: Go compiles generic methods per GC shape with a dictionary
argument; `SetAt` (bounds check, uniqueness check, out of line copy
call) costs 113 and `M_push` 179 against the inlining budget of 80, and
a call counts about 57. Their pointer receivers also take the address of
the caller's list, which keeps a local list in memory (loads and stores
on every iteration) instead of registers.

Fix: generated code uses small value receiver forms that each inline
and never take the list's address: `if !xs.Mine() { xs = xs.Unshared() }`
then `xs.PutM(i, x, at)` (or `xs.PtrM(i, at)`), and `if xs.CanPush() {
xs = xs.PushFast(x) } else { xs = xs.PushSlow(x) }`. Faults are panics of
`*arithFault` as in `AtM`. Check with `go build -gcflags=-m=2` in std/rt
(`can inline (*List[go.shape.bool]).PutM with cost 28`). Sieve went from
0.36 s to 0.15 s.
