# 0291: A native Map whose copies cost O(1) (the Go backend's log and fork)

Status: Proposed (2026-10-20, Xo Lang 3; for the lead and the owner)

## Context

The spec calls `Map[K, V]` an immutable persistent hash map with value
semantics (core 2.1). On the native backends (decision 0150 stage 3) a
Map is a struct of copy-on-write lists (keys, values, live flags, an
open-addressing table): a write to a Map that another value still shares
copies all of it, O(n).

That made the large-heap comparison (owner-approved 2026-10-19) never
finish: with `Mutex.with`, the packed closure keeps the value it was given
(ssa build.xo pack_orig) so that an error or a fault hands it back
(decision 0085), the Map is shared while the body writes it, and every
`put` copies the table. Measured 2026-10-20 (--release arm64, Mac), per
put inside `Mutex.with(fn(var m) { m.put(..) })`: 23 us at 10,000
entries, 170 us at 40,000 (a plain put: 38 to 52 ns at any size). The
bench store preloading users (after its own email scan was fixed,
d294c50b): 98 us per insert at 10,000 users, 280 us at 30,000; Go's
server 24 to 42 us flat. The Go backend's `with` stays flat (about 700 ns
per put at 10,000 and 40,000 entries).

The same cost hits any shared Map write: a Map copied into a struct and
changed, a Map captured by a closure and changed, a Map read by
`Mutex.with_read` while a writer changes it.

Today's costs (n = 200,000, Int keys, --release, two runs):

| | put | get | iterate, per entry | copy, then put | put, Str keys |
|---|---|---|---|---|---|
| native (lists) | 33 to 34 ns | 5 to 6 ns | under 1 ns | 110 us | 92 to 99 ns |
| Go backend (log and fork) | 42 to 51 ns | 14 to 15 ns | 1 ns | 93 to 98 ns | 104 to 106 ns |

## Decision (proposed)

Port the Go backend's Map representation (std/rt/map.go) to the native
backends, in place of the four lists:

- **A value is a header (table, view length, live count).** A table is an
  append-only log of entries (key, value, the previous entry of the same
  key, the origin that gives the key its insertion position; a removal is
  a tombstone) plus a hash index from each key to its newest entry.
- **Ownership by claim, as List's backing array:** a put through a header
  whose length is the table's claimed length takes the next position with
  a compare and swap and appends in place (amortized O(1), no copy). A
  stale header (another copy appended first) forks: a new table whose
  parent is the stale view, O(1); lookups search the fork's index, then
  the parent view by the prev links. Forks nest a bounded depth; deeper,
  or when overwritten entries and tombstones outnumber live keys, the view
  is flattened into a fresh table in O(n), amortized against the appends
  that made the garbage.
- **Soundness** is std/rt/map.go's argument (entries are never written
  after they are appended; the index is not part of any view; at most one
  value appends at a position), and its model tests (std/rt
  map_model_test.go: random operations on copies against a reference) are
  ported to a private native test.
- **RC interplay:** a header is a counted record like a List's; entries
  hold counted keys and values, released when a table's last header goes
  (a fork holds its parent view). `Mutex.with` keeps its rollback
  snapshot unchanged: holding the old header now costs O(1), and the
  body's puts append (the first one forks only if the snapshot header
  claimed the log, which it does not: the moved value claims).
- **Codegen:** the xo.map.* functions generated per type (compiler/ssa
  map.xo) are rewritten against the new layout; the hash functions per
  type stay. JSON, Debug, equality, iteration order and every Map method's
  results are unchanged (golden parity with the Go backend, which already
  has this representation, is the behavior test).

## Cost

Estimated from the Go backend's figures and the native ones: a put gains
the claim's compare and swap and the log append (about 33 to 45 ns), a
get on an unforked table stays one index probe plus one entry load (5 to
8 ns), a get through a fork follows prev links (bounded by the fork
depth), iteration walks the log skipping overwritten entries and
tombstones (under 2 ns per entry while garbage stays under live keys).
Copying a Map and writing it goes from O(n) to O(1). To be measured on a
prototype before the switch: bench/perf/compute and the json benches
(Map-heavy decode), callgrind's CRUD scenarios, xo's own build (the
compiler is a heavy Map user: a regression there shows in every build),
and the cpuburst and large-heap runs.

## Consequences

- Shared-Map writes stop being O(n): `Mutex.with` over a large Map, Maps
  in structs that are copied, closures capturing Maps. The large-heap
  comparison then measures the runtime.
- One representation on every backend (Go and native), one soundness
  argument and one model test, kept in step.
- A Map's memory holds overwritten entries until it is flattened (at
  most about twice its live entries, as on the Go backend).
- The compiler itself changes speed (it uses Maps throughout); the
  fixed point and the cold-build measurement decide whether the switch
  lands as is or with a faster path for unshared Maps.

## Alternatives

- **A HAMT** (hash array mapped trie, with an insertion-order structure
  beside it): O(log n) copies and writes, but every get becomes several
  dependent loads, insertion order needs a second persistent structure,
  and it would differ from the Go backend's proven design.
- **Drop the rollback snapshot in `Mutex.with`** (decision 0085's
  fault behavior; an owner choice the lead has taken to the owner as a
  possible interim): fixes this pattern only; every other shared-Map
  write stays O(n).
- **Keep the lists and document the cost**: contradicts the spec's
  persistent map and leaves a quadratic trap in the most common service
  shape (a store under a Mutex).
