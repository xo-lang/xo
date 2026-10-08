# List writes in place rely on own marks at every copy site

Status: Observed (2026-10-08, internal/gen/own.go, std/rt/list.go)

Symptom: none in a correct build; this is how the List ownership scheme
breaks if a change misses a case. A list changed through one value shows
up in another value (value semantics broken, core 5.1), silently, or a
`-race` run reports a data race on `listBlock.used`. Two such holes were
found while building the scheme (2026-10-08), before any test hit them:
the old `List.With` wrote in place through a value receiver copy, and a
user method's `var` receiver on an element (`grid[i].method()`) was
copied into a temporary without a mark, so a write through it could
reach a copy of `grid` that a by value argument of the same call shared.

Cause: a List writes its array in place while its block is not marked
shared, so every place that makes a second header for a block must mark
it. Go copies headers silently (assignment, captures, struct copies,
channel sends), so the generated code marks at the source: every read of
a binding, field, or element whose type holds lists is wrapped in an own
mark unless the reading expression asked for a borrow, results of std
calls are marked unless the producer is known fresh, closures mark what
they capture, interface conversions mark. The runtime marks what it
shares or copies (`SliceRange`, `Concat`, `markElems`, `Mutex.with`,
`Map.update`). Nested lists are marked lazily, so a header taken out of
an element must never be written while another header of its outer list
exists unmarked.

Fix / rules when changing codegen or rt:

- A new expression kind needs nothing: reads mark by default. A new
  borrow (calling `borrow`/`borrowAs` instead of `val`) needs a
  `borrowOK` check over every operand evaluated while the copy is alive,
  and the copy must not be stored.
- A new std function that returns a list (or a value holding one) is
  marked by default; add it to `freshStd` only if it returns fresh lists
  and calls `markElems` on elements it copied from elsewhere. A new read
  only List method may join `borrowRecv` if it copies nothing out, or
  marks what it copies.
- A runtime function that returns a header sharing an argument's array
  must call `markShared`; one that copies elements between arrays must
  call `markElems`; one that hands a stored value to user code by pointer
  must `MarkP` it first.
- A generated write through an element must make every list on the path
  unique first (`uniqueList`, then `PtrM`), never write through a header
  read out with `AtM`.
- Check with `go test ./internal/gen -run 'TestListModel|TestGolden'`
  (`XO_LIST_SEEDS=40` for more programs) and the `-race` runs: removing
  any single rule above makes one of them fail.
