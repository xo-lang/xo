# Map copies: the first write decides who appends in place

Status: Observed (2026-10-08, bench/perf and std/rt map tests)

Symptom: with the persistent trie (before 2026-10-08) an update heavy Map
loop cost 720 ns and 1.7 KB per `put` (45x a Go map). The obvious fix,
transient trie nodes owned by "the" binding, is unsound in Go: assigning a
struct, capturing a value, or sending it on a channel copies the header
silently, so two headers would own the same mutable nodes. A second
surprise: the old trie was not O(log n) for "many one key edits of one
map" either, because its insertion log was a List and a stale List push
copies the whole backing array (117 µs per edit of a 50k map).

Cause: Go gives no hook on value copies, so ownership cannot be tracked
per binding without touching every copy site in codegen (assignment,
arguments, returns, captures, struct and collection literals, channel
sends, `Mutex.with`).

Fix: the runtime claims ownership the way List already did (std/rt/map.go,
soundness argument in the comment on `Map`). A map is an append only entry
log plus a hash index; a header sees a prefix of the log. A write through
a header whose length equals the table's claimed length (`used`) appends
in place after a compare and swap; any other header is stale and forks a
child table in O(1). Entries a header sees are never written again, so no
copy can observe another's change, whoever writes first.

Consequences to know when working on rt or reading profiles:

- After `let b = a`, the first of `a` or `b` to be written keeps the fast
  path; the other forks on its first write (O(1), then lookups also search
  the parent view, up to 8 fork levels before an O(n) flatten).
- A stale snapshot keeps the whole table alive until the owner compacts
  (the log is flattened when it holds more garbage than live keys, or when
  a fork's own log outgrows the map).
- `Map.update` with a closure that fails after the fast path probe still
  leaves the map unchanged; if another copy claimed the log meanwhile, the
  write falls back to claim and record.
- `Map.update` allocates one word per call: the closure gets `&v`, which
  escapes because the closure is an unknown function value.
- Index slots, the index pointer, the log array pointer, and `used` are
  atomics because a snapshot may be read on another task while the owner
  appends. `TestMapConcurrentReaders` and the `map_value_semantics` golden
  (in `TestGoldenRace`) run this under `-race`.
