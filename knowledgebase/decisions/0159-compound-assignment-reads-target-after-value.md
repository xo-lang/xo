# 0159: A compound assignment reads the target's old value after the value

Status: Accepted (2026-10-11). Implemented in `internal/ssa/build_stmt.go`
(`assign`) and `selfhost/ssa/stmt.xo` (`assign`), spec 5.2. Test: the
golden `internal/llvm/testdata/golden/compound_assign_call` (run by the
LLVM and arm64 golden tests). Found by the full tier of TestSelfhostArm64
(decision 0158).

## Context

Decision 0140 makes an assignment evaluate the indexes of its target,
then the value, then store, keeping what the value changed in the
target's binding. The SSA builder (both native backends) lowered a
compound assignment `t.f += e` by reading the old `t.f` first: the place
cached `t` before `e` ran, and the store wrote that cached `t` back, so
a `var` call of `t` in `e` lost its changes. `g.n += g.bump(s)` printed
`compound 6 5 k4` on LLVM and arm64 and `compound 6 6 c0` on the Go
backend. On the LLVM backend a large `var` parameter lives in its home
(decision 0148), which the call had updated in place, so the cached `t`
aliased the new one and its fields were released while in use: the LLVM
build of the self hosted arm64 emitter (`g.mem_bytes += 8 *
g.nwords(t)`) computed frames 256 bytes too small in 33 runs, and a
reduced program crashed.

## Decision

A compound assignment evaluates the target's place (its indexes), then
the value, then reads the target's old value, then stores: the value's
changes to the binding are kept, as with `=` and as on the Go backend
(Go evaluates the call before it loads the operand).

## Consequences

- The three backends agree on compound assignments whose value changes
  the target's binding; the golden compound_assign_call covers it.
- `t.n += t.bump()` where bump changes `t.n` itself adds to bump's `n`.
- The SSA of every compound assignment makes the value before the read
  of the old value (value numbers in the SSA goldens move).

## Alternatives

- **Read the old value first and re-read the binding for the store**:
  keeps the call's other changes but adds to an `n` the call may have
  changed, unlike the Go backend.
