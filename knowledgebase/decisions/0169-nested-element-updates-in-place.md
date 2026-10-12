# 0169: Nested element updates in place on the native backends (`list.take`)

Status: Accepted (2026-10-11), implemented in `internal/ssa`
(build_stmt.go `place`, rc.go, landing.go), `internal/llvm/fromssa.go`
(`list.take`, `hole`), `internal/arm64/isel.go` (`list.take`,
`storeHole`), and their mirrors `selfhost/ssa` (stmt.xo, rc.xo,
defer.xo, passes.xo, map.xo) and `selfhost/arm64` (calls.xo). Tests:
golden `internal/llvm/testdata/golden/nested_update` (TestGolden on
LLVM and arm64: the Go backend's output, no allocation live),
`internal/arm64/testdata/run/nested_update` (TestRun),
`TestNestedUpdateInPlace` (internal/llvm: the golden makes at most 400
allocations on each native backend), the SSA golden `rc`. Evidence:
`bench/results/nested-update-2026-10-11.md`. Builds on decision 0046
(reference counting and reuse) and closes the case decision 0147 left
open ("list elements under a borrowed root").

## Context

On the arm64 and LLVM backends an update of a collection inside a list
element (`g.blocks[g.cur].insts.push(x)`, `xs[i][j] = v`,
`xs[i].f[j].g = v`, `xs[i].m[k] = v`, `xs[i].s += "x"`) copied the whole
inner collection on every update (gotchas/xo-native-nested-element-update.md).
The SSA builder lowered the place as a read of the element
(`list.index`, a projection that borrows from the list), the update of
the inner collection, and a store back (`list.set`). The projection
holds a second reference to the inner collection while the list's slot
holds the first, so the inner collection's count was two at the update
and `reuse` copied it. The Go backend updates in place. The self hosted
compiler worked around it by hand (selfhost/arm64 take_insts and
put_insts, a memo kept out of its list, push_rng and push_int), and the
first arm64 build of selfhost/xo without them ran over 20 minutes.

## Decision

- A new runtime operation of the SSA IR, `list.take(List[T], Int) ->
  (List[T], T)`: element i moves out of the list, and its slot holds a
  placeholder until `list.set` stores a value back. It is a list update
  (it consumes the list, needs it unique, so `reuse` comes before it
  when the list is not provably unique; the list it returns is fresh,
  like `list.pop`'s), it faults when the index is out of range (Pos)
  before it takes anything (a landing releases the list, keepsOperands).
- The placeholder is what reference counting can release, retain, or
  share without effect: Str, List, Box, and handle leaves point to the
  empty string's object (immortal: retain and release never change its
  count), a function value has a null environment, and an enum's tag is
  past its variants (the helpers' switch on the tag takes no case).
  LLVM stores a constant (`hole`); arm64 stores only the words counting
  reads (`storeHole`: one address, tags, environment words).
- The SSA builder reads a list element on the way to a store with
  `list.take` instead of `list.index` when the element type has heap
  leaves: every level of a place (`xs[i].f`, `xs[i][j]`, map puts
  inside elements, `var` receivers and arguments, List push/set/pop,
  StrBuilder writes), so the element's own collections are unique and
  update in place. The store back (`list.set`) carries no position: the
  take checked the index of the same list.
- Value semantics do not depend on where the store happens: the take is
  an update of the list's SSA value, so a list still used afterwards (a
  copy kept, `let before = g.blocks`, or the old value read on a path
  where the store does not happen) is retained, and `reuse` copies it
  before the take; the copy holds the element, the taken list the
  placeholder.
- A place keeps `list.index` (`place.keep`) where the store may not
  follow and the old list would stay live, which would copy the outer
  list at every take: `Map.update` (stores only when the closure
  succeeds), and a compound assignment whose operand can leave the
  statement or read a list element (`plainOperand`: literals, names,
  fields, and operators over them move; calls, `?`, blocks, and index
  reads keep).

## Consequences

- Micro benchmark (bench/perf/nested, 20,000 updates): 20,000 pushes
  through a `var` receiver 63,580 us to 104 us on arm64 (39,993
  allocations to 18), `rows[0].cells[j].g = v` 1,477,240 us to 292 us;
  a shared outer list still copies at every update (5,998 allocations
  before and after).
- The switch-over driver with selfhost/arm64's take_insts and put_insts
  rewritten as direct nested updates compiles selfhost/xo as fast as
  with them (273.1 s against 276.4 s, 1.2% fewer instructions, the
  same assembly): the workaround is no longer needed. The decision does
  not speed up the compiler that keeps its workarounds, so the factor
  of about 2 against the Go built compiler lies elsewhere.
- The golden program makes 159 allocations on each native backend,
  2,162 before (`XO_LEAKCHECK=1`).
- selfhost/ssa and selfhost/arm64 lower `list.take` the same way:
  identical SSA dumps and assembly on the new runs.
- A by value receiver does not make list writes copy on the native
  backends (borrow inference passes it without a count; 28 allocations
  for 20,000 alternating `peek`/`put` calls on a two list state struct,
  scratch program): the by value receiver trap is the go backend's
  (std/rt shared marks).
- Not done: contents of a box (a field of a recursive type) under an
  element still copy (the box is replaced by a new one); `Map.update` on
  a map inside an element keeps the projection; LSR (decision 0054) does
  not follow a list through `list.take` (the buffer does not move, so it
  could).

## Alternatives

- **Pair `list.index` and `list.set` in the RC pass** (load without a
  retain, store without releasing): no backend change for the load, but
  the slot is stale between the two, and a fault landing or a cancel
  check between them (FaultUnwind adds them after every faulting
  operation) would release the list with the moved element still in it.
- **Zero the slot and make the release helpers skip null**: a branch in
  every release of every program for one case.
- **Leave the source to move out and back by hand** (the gotcha's fix):
  every author pays for it, and it was missed in the compiler itself.
- **A placeholder built from ordinary values** (`[]`, `""`, a unit
  variant) stored with `list.set`: an allocation per updated list leaf,
  and impossible for handles.
