# 0142: A `var` receiver or argument is read after the call's arguments

Status: Accepted, implemented (2026-10-10). Spec: core 5.2. Code:
`internal/ssa/build_call.go` (`callAsVia`: the places of a `var`
receiver and of `var` arguments are built in order, their values read
after every argument; the `push` and `set` of List and Buf evaluate their
arguments before reading the list), `internal/ssa/build_map.go`
(`mapUpdate`). Test: `internal/gen/testdata/golden/var_recv_after_args`
(Go backend, and listed in `internal/llvm/testdata/gen_native.txt` for
LLVM and arm64). Found by the self hosted checker, stage 2 (decision
0141). Follows decision 0140.

## Context

`c.implements(t, c.named("Display"))` in selfhost/check, where `named`
pushes onto the type table `c.tys` and `implements` (a `var` method)
reads it, worked on the Go backend and faulted with an index out of
range on LLVM and arm64: the native lowering read the receiver `c` when
it built the call, before the arguments, so the method ran on the value
without the push, and the receiver written back after the call dropped
it. `xs.push(f())` where f pushes onto `xs` lost f's push the same way.
The spec did not say in which order a call reads a `var` receiver.

## Decision

A call evaluates its receiver's place (the indexes of `t.items[i]`) and
then its arguments in source order; the value of a `var` receiver and of
each `var` argument is read after every argument is computed, and the
changed values are stored back after the call. Changes the arguments
make to those bindings are kept: `t.get(t.add(7))` sees the item `add`
pushed. This is the rule of decision 0140 for an assignment, applied to
the places a call writes.

## Consequences

- The Go and native backends agree. The SSA of a `var` method call
  reads the receiver after its arguments (the `equality` and `rc` SSA
  goldens changed by reordering only).
- Two `var` places of one call that overlap stay an error (XO0503,
  decision 0111), so the order in which they are read does not matter.

## Alternatives

- **Read the receiver first (the old native order)**: drops the
  arguments' changes silently, as decision 0140 found for assignments.
- **Forbid arguments that change the receiver's binding** (a checker
  rule): rejects `c.f(c.g())`, the common shape of table building code,
  which has one obvious meaning.
