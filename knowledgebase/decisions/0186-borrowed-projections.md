# 0186: Loop elements and list element reads borrow from their container

Status: Accepted (2026-10-12). Implemented in `internal/ssa/rcborrow.go`
(`borrowProjections`, called by `InsertRC` per function) and its mirror
`selfhost/ssa/borrow.xo` (`borrow_projections`, from `insert_rc`); the
rc stage of both is byte identical. Tests: LLVM golden
`internal/llvm/testdata/golden/borrow_proj` (run by the LLVM and arm64
golden tests, shared and unique containers, the Go backend's output).
Refines decision 0046 (Perceus RC) and extends the borrow inference of
0149 from parameters to projections. Evidence:
`bench/results/rc-borrow-2026-10-12.md`.

## Context

The RC pass (rc.go) gives a projection out of a heap container
(`list.index`, `list.get`, `payload`, `capture`, `unbox`) its own count:
retained after its definition when it lives on, released after its last
use. Only a projection out of a borrowed parameter or a constant was
borrowed (never counted). So `for inst in g.blocks[b].insts` over a
`var` receiver duplicated every heap field of each 200 byte MIR `Inst`
(eleven counts in another cache line) and dropped them at the end of
the iteration, and `out[w] | live_in[ti][w]` duplicated and dropped the
inner list per word, because the landing check of the bounds checked
index split the use from the definition and the in block cancellation
could not pair them. bench/results/xo1-perf-2026-10-12.md (section 4)
measured 18.0% of xo1's samples in the rc helpers and 26.6% in
allocation, and fixed the two worst loops in the source by passing the
lists to helpers, where parameters are borrowed.

## Decision

A projection root `v` with a container (operand 0) is **borrowed** when
both hold:

1. **No terminator uses it**: it is not returned, passed on an edge
   (a block argument, so not carried around a loop), or given to a
   fault. Every use is an ordinary instruction.
2. **Its owner is not consumed while it is alive.** The owner is the
   container's leaf, or, when the container is itself a borrowed
   projection, that projection's owner (so `g.blocks[b].insts[i]` owns
   through `g.blocks`, the leaf of the `var` receiver). Over `v`'s live
   range (from its definition to its last use, on every path, by a
   backward walk from the blocks of its uses to its defining block), no
   instruction consumes an operand whose root is the owner's root, and
   no edge passes one. Consuming covers every in place update and every
   move: `list.set`, `list.push`, `list.pop`, `list.take` (decision
   0169's nested updates take the element out of the container),
   `setfield` of a struct holding the container (the `var` binding's
   `g.n = ...`, including one that replaces the container's own field),
   a `var` call (`g.note(...)`: the callee may update its parameter in
   place, and on LLVM a large `var` parameter lives in a home the call
   writes, decision 0148), `reuse`, a constructor, and a block argument.
   The test is per root, not per leaf (conservative).
3. **A consume of it does not outlive its owner's own uses.** Where an
   instruction consumes the projection (a push of the element into
   another list, an owned argument), some path from there must still
   use the owner's root by its own uses. Otherwise the owner would have
   died before the consume and left the value unique (a list handed on
   and then updated in place), and borrowing would keep the owner alive
   and the value shared, so the callee's update would copy it
   (`last_long(g.blocks[0].insts)` as `g`'s last use: 3 more
   allocations in the golden before this rule). This rule is about
   cost, not soundness.

A borrowed projection is never retained after its definition or
released after its last use; a use that consumes it (a push of the
element into another list, an owned argument) retains it first, as for
a borrowed parameter. Every use of it (of any of its leaves, through
field aliases) also borrows its owner, which extends the owner's
liveness to the projection's last use: the owner is released after it,
never before. Retains and releases of other values do not matter: the
owner is held by the function throughout, so its count is at least one
and any in place update elsewhere through another reference sees a
shared count and copies.

Interaction with evaluation order (decision 0159): a compound
assignment `g.n += f(inst)` evaluates the value, then reads and stores
`g.n`; the store is a `setfield` of `g`, which consumes the owner's
root, so an element of `g` alive across it keeps its own count. Nothing
changes in the order of evaluation; the SSA is the same except for
counting instructions.

Soundness: the borrowed projection lives no longer than the owner (rule
2 plus the owner's extended liveness) and never aliases a mutation of
it (rule 2: every mutation consumes). The RC verifier (verify_rc.go,
verify.xo) needs no new rule: it checks a projection held zero times as
alive while its container is held, which is what the decision
guarantees, and every list update still gets a list held once.

## Consequences

- The loop element of `for x in <place>` and a read through a list
  element (`a[i][j]`, `xs[i].f.len()`) cost no count traffic when the
  body only reads the container: the source workarounds of
  xo1-perf-2026-10-12.md (helpers taking the list as a parameter) are no
  longer needed for new code.
- An owner may be released later than before (after the last use of
  what it lends), which can raise peak memory slightly.
- The decision is per function and linear in the uses of each
  projection; the rc stage of selfhost/ssa matches the Go pass byte for
  byte, so the fixed point of xo1 holds.
- xo1 compiling selfhost/xo executes 32.2% fewer retains and 40.3%
  fewer releases (6.31 to 4.28 billion, 5.05 to 3.02 billion), 2.2%
  fewer instructions and 4.1% fewer cycles, with the fixed point intact
  (the Go compiler's and both xo1s' assembly byte identical); peak
  footprint 18.4 to 19.3 GB (rc-borrow-2026-10-12.md). bench/perf/nested
  walk: 600,041 retains to 1.
- No allocation count goes up on the golden borrow_proj case by case
  (rule 3), and retain and release counts go down or stay.
- Loops whose body changes the container (pushes, nested updates, `var`
  calls on the receiver) keep the old counting; making those borrow
  would need the element read again after the change.
- Addendum (2026-10-19): a pair loop, `for (i, x) in xs.indexed()` or
  `for (k, v) in m`, binds the tuple pattern's two parts apart
  (compiler/ssa stmt.xo, map.xo) instead of building the pair: the
  tuple consumed the element, so it was retained and released on every
  iteration where `for x in xs` borrowed it. A pattern that keeps the
  pair (`for p in xs.indexed()`) still builds it. xo's `__text` 32.65 to
  32.24 MB (-1.25%); golden loop_pairs.
- Addendum (2026-10-30): rule 3 asks about the owner's leaf, not its
  root: a use of a leaf that holds it or that it holds counts, a sibling
  field's does not. And a path that reaches the root's own block always
  ends there, also when the consume is in that block (the root is made
  again before any use on the way back). Mutex.with's packed path pops
  the value out as a `(List[T], T?)` pair, hands the payload of field 1
  to the closure and pushes onto field 0 after it; the push, and the
  payload's own read reached around the loop, made the payload a
  borrowed projection, so the caller kept a reference and the closure's
  first push copied the list: 200,000 pushes 31.4G instructions, now
  120M (internal/arm64 TestMutexWithPushLinear, golden conc_mutex_move,
  gotchas/mutex-with-borrowed-payload.md).

## Alternatives

- **Borrow in the builder** (a `for` over a place that the body does not
  write reads elements without a projection): the builder does not know
  what the RC pass will consume (borrow inference of callees), and the
  rule would be duplicated for `a[i][j]`.
- **Cross block cancellation** of retain/release pairs: needs the same
  live range and touch analysis, then removes code the walk inserted;
  borrowing first is simpler and also removes the edge releases on
  landing pads.
- **Per leaf touch test** (only the container's own leaf, not every leaf
  of its root): would borrow across `g.n = ...` in the loop body; left
  out because a `setfield` of a `var` binding on LLVM can write the home
  of the whole struct (decision 0159's bug class).
