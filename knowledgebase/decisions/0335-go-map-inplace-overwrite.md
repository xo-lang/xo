# 0335: The Go backend's Map overwrites a key in place while unshared

Status: Accepted (the lead, under the owner's overnight delegation,
2026-10-22)

## Context

The Go backend's Map (std/rt map.go) is a persistent log: a put appends
an entry, a copy shares the table, and the first put after a copy
decides which copy keeps appending (the other forks). Overwriting an
existing key appends too, and once the log holds more than 2n+32 entries
it is flattened into fresh arrays. A map written much more often than it
grows flattens again and again: bench/perf's putget (3M puts and 3M gets
over 50k keys) and update (3M Map.update over 50k keys) flatten about 60
times a run, which with the allocation and the returned memory is most
of their time (profile in the lead's notes,
go-backend-map-profile-2026-10-22.md). A scratch runtime that overwrote
the newest entry in place ran putget 39% and update 38% faster with the
same checksums, but was unsound: a copy of the map shares the entry.

Lists already solve the same problem (std/rt list.go): a block carries a
sticky `shared` flag, the generated code marks a list wherever it copies
one out of a place into something that outlives the read (internal/gen
own.go, now compiler/gen own.xo), reads that end before the place can be
written again are borrows, and a write through an unshared block goes in
place.

## Decision

- The table carries the same sticky `shared` flag. `Map.XoMark` and
  `Map_Own` set it, and gen treats a Map as it treats a List: it holds a
  markable header (holds_list, mark_stmts, own_code), so every own site
  (a copy into a binding, field, collection, capture, return, channel
  send, std call argument; a struct, tuple, Option or Result holding a
  map through its XoMark) marks it. The receivers of the read only Map
  methods (len, is_empty, get, get_or_fault, contains, keys, values,
  entries, filter, map, any, all) are borrows, as the read only List
  methods are: none keeps the receiver's table past the call.
- Returning a Map binding is its last use and takes no mark when the
  binding is a local of a named function's body (not a parameter or
  receiver, whose table the caller may still hold), outside block
  closures, in a function with no `defer` (map_last_use in own.xo). So a
  map a helper builds and returns still overwrites in place in the
  caller. Lists keep marking at a return (unchanged).
- Map_write overwrites the value of a key's newest entry in place, and
  gives back the claimed log position, when the table is a root (no
  parent), not shared, the entry is live, and the write is a put (not a
  remove). Map.update's owner fast path takes the same branch. Removes
  still append tombstones; forks and shared tables still append.
- Flattening and forking produce fresh tables, so a map that was copied
  once overwrites in place again after its next flatten.
- The runtime marks the maps it copies or hands out: Map_With, the
  request and response `with_header` (a copy of the receiver),
  testing's FakeEnv.vars (under the lock, since FakeEnv.set overwrites).
  Set is unchanged in gen: a set never overwrites a key, so it never
  takes the in-place path (Set.XoMark exists for runtime callers).

Soundness is List's invariant U for tables: an unshared table is
reachable from exactly one live header, so no other header can observe
an overwrite, and the log's points 1 to 4 (map.go) hold for every header
that exists. A shared table is only appended to, so concurrent readers
of a shared map still race with nothing.

Scope: the Go backend only. The native and LLVM backends' maps are
reference counted (decision 0291) and already write in place when the
count is one.

## Consequences

- putget and update on the Go backend no longer flatten (measurements in
  the commit message and the lead's notes); branch and copymut, which
  copy maps, keep their cost (one flag store per copy).
- A copy marks the source's table for good: until its next flatten the
  original appends on overwrite, as every map did before.
- Hand written runtime code that copies a Map header and then puts an
  existing key on one of the copies must mark first (Map_Own), as it
  must for a List. std/rt's model tests (map_model_test.go) mark at
  each copy, the way generated code does.
- Generated Go for programs and std modules whose types hold maps gains
  XoMark calls (std/rt std_gen_*.go regenerated).

## Alternatives

- Flatten at 4n instead of 2n+32: fewer flattens, more memory, still an
  append per overwrite; kept as a later option.
- Reuse the old log's array on flatten: needs the same uniqueness
  question answered.
- Ownership by header address in the table: a Go struct copy keeps the
  address of neither header, so a copy that only reads cannot be told
  from the owner. Rejected.
