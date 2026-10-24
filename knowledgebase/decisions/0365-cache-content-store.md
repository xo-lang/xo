# 0365: A content store behind the back-end cache's per-id keys

Status: Proposed (Xo Lang 3, 2026-10-24; for the owner's Sunday review,
nothing built)

## Context

The back-end cache keys every entry with the code generation key (cache_id,
bcache.xo: a hash of the backend's sources), so xos built from different
backend sources never share an entry. That is the safety property: a
different compiler is never served another compiler's bytes. Its cost, seen
in the store-log (ADR 0351 addendum): the cache stores 2,454 GB in 54.6 hours
at a 16 GiB cap, so it holds about 8.5 minutes of stores; the newest 16 GiB
came from 5 live ids storing at once (6.5, 5.5, 2.6, 1.7, 0.9 GB). Idle-id
eviction would free nothing (no bytes in that window from an id idle for 30
minutes; prune is already oldest-used first), so the lever is the volume each
live id stores.

**Measurement** (2026-10-24; scripts in the session scratchpad, results in
~/xo-lead/notes/lib1/resume.md): XO_CACHE_STATS=keys logs a sha256 of every
loose write and of every entry inside each pack segment (keys mode only;
cachestats.xo). Clean committed xos X1 and X2, one private cache, whole-build
reuse off, X2 differing from X1 only in compiler/arm64, so that it has its own
id and misses every entry. The bytes X2 stores that are byte-identical to
something X1 stored, for a cold build of compiler/xo (1,120 MB for X1):

| X2 differs by | X2 stores | identical to X1's | fn | inl | rc | obj | part |
|---|---|---|---|---|---|---|---|
| a comment | 1,833 MB | 899 MB (49%) | 100% | 100% | 100% | 100% | not yet stored by X1 |
| one peephole off (borrow_flag_peep) | 1,833 MB | 897 MB (49%) | 100% | 100% | 100% | 99% | not yet stored by X1 |
| copy_runs off (every function changes) | 1,887 MB | 489 MB (26%) | 31% | 100% | 100% | 2% | 0% |

(X2 stores 700 MB more than X1 in its cold build because X1's part packs form
on its second build; on the second build, with equal code, the part pack is
identical too: 713 MB of 713 MB.) http_server: 42%, 40% and 32% of a cold
build. The front end (fe) and the inliner and reference counting entries
(inl, rc) come before the backend, so a backend change shares them whole;
fn, obj and part share what the change does not touch.

So the second id re-stores the first's bytes at 26% (a change to every
function) up to 100% (a change to a few), while no entry could be shared by
key.

**Read cost** (uncached disk, files written with F_NOCACHE so the page cache
holds none, 8 KB bodies, 20,000 entries): a direct hit reads 115 microseconds
on one thread (26 at 8 threads); a hit through a pointer file (read the
pointer, then the body) 215 (45 at 8 threads). sha256 hashes 3.0 GB/s.

## Decision

Proposed, not built. Keep every key per id; store the bytes once by content.

- An entry's file in the id's key space becomes a pointer (the content hash,
  32 bytes) for bodies over a threshold (small entries stay inline); the body
  lives once in `objects/<xx>/<hash>`.
- A store hashes the body; when `objects/<xx>/<hash>` exists it writes only the
  pointer (no body write, which is the saving), else the body (temporary name
  plus rename, as cache_put) then the pointer.
- A hit reads the pointer, then the body, and checks the body's hash only in
  tests (the body is addressed by its hash, written whole by rename).
- Pack segments hold the pointers for their entries (or inline small bodies);
  the part packs, whose bodies are the 713 MB of compiler/xo, point into the
  object store.
- Prune: pointers are entries as today (oldest used first, KEEP, the cap). A
  body is removed only when nothing points at it, so a prune also sweeps the
  bodies: it walks the pointers it keeps (it already walks them), builds the
  set of live hashes in a bounded pass (sliced like the shard walks), and
  removes the bodies not in the set and older than KEEP. A body is always
  written before its pointer, and a body younger than KEEP is never removed, so
  a store racing a sweep keeps its body.

## Consequences

- Stored bytes fall by the shared share of the 2,454 GB: between 26% and 100%
  of what a second id stores, so the 5 live ids of the store-log hold much
  more history in the same cap.
- Each hit through a pointer costs one more open and read: +100 microseconds on
  one thread uncached (+19 at 8 threads), a hit in the page cache a few
  microseconds. The compiler reads about 6,000 entries for hello, but through
  the shared pack (no per-entry opens); the pointer cost applies to loose
  entries (obj, a few thousand) and to pack hits only when a segment points
  out. Measure before building.
- A store hashes its body: 1.1 GB for a cold compiler build is 0.4 s of one
  core at 3 GB/s, against the 25 s it takes.
- Prune gets a reference sweep, the one real complexity; a bug there loses
  entries (a miss, never a wrong hit: bodies are addressed by their hash).
- Safety is unchanged: a key belongs to one id, and the pointer's body is
  addressed by the hash of its bytes.

## Alternatives

- Key entries by their inputs only (no id): shares everything and is wrong
  whenever the backend's output depends on code the key leaves out. Rejected
  (the property the id exists for).
- Hard links for equal bodies: needs the hash anyway and breaks on a volume
  without links (Windows CI).
- Compress segments (zstd): shrinks every id's bytes by a constant, shares
  nothing; additive to this, measured separately.
- Do nothing: the cap stays 8.5 minutes of stores for 5 ids.
