# 0140: An assignment computes its value before it reads the place it stores into

Status: Accepted, implemented (2026-10-10). Spec: core 5.2. Code:
`internal/ssa/build_stmt.go` (`place`: the list of an index place is read
when first used). Test: `internal/gen/testdata/golden/assign_rhs_mutates`
(Go backend, and listed in `internal/llvm/testdata/gen_native.txt` for
LLVM and arm64). Found by the self hosted checker (decision 0139).

## Context

`c.objs[o].tparams = c.new_tparams(tps)` in selfhost/check, where
`new_tparams` pushes onto `c.objs`, kept the pushes on the Go backend
and lost them on LLVM and arm64: the native lowering read the list `c.objs`
when it built the place `c.objs[o]`, before the value, then stored the
changed element into that stale list and wrote it back over the one the
value had grown. The program then faulted with an index out of range.
The spec did not say in which order an assignment evaluates its parts.

## Decision

In `place = value` (and `place op= value` for the store), the indexes of
the place are evaluated first, then the value, then the place's
collection is read and the element or field is stored into it. Changes
the value makes to the place's root binding are kept. An index out of
range faults after the value was computed, as on the Go backend.

## Consequences

- The Go and native backends agree; the SSA of a plain `xs[i] = v`
  reads `xs` after computing `v` (the rc golden changed by reordering
  only).
- A compound `xs[i] += f()` reads the element before `f` runs and stores
  into the list read then, as before.

## Alternatives

- **Forbid a value that mutates the place's root** (a checker rule like
  XO0503): sound, but rejects a common pattern of table building code
  (`t.objs[o].kids = t.add_kids()`), which has one obvious meaning.
- **Read the place first (the old native order)**: drops the value's
  changes silently.
