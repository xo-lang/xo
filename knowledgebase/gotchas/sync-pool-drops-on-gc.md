# sync.Pool drops its items at every GC, so it barely reuses under heavy allocation

Status: Observed (2026-10-08, request scoped arenas, bench/perf/http)

Symptom: the first version of the request arena free list used
`sync.Pool`. Under load the server still allocated arena slabs on almost
every request (`arenaSlice` near the top of the allocation profile), and
the arena build allocated 9.6 KB per request instead of the 8.5 KB it
reached once reuse worked.

Cause: a `sync.Pool` moves its items to a victim cache at each garbage
collection and frees the victim cache at the next one. The Xo CRUD server
ran hundreds of GC cycles per second (a 4 MB heap goal and about 10 KB
allocated per request), so a pooled arena survived only a few hundred
requests, about as many as were in flight.

Fix: keep reusable per request memory in a structure the GC does not
clear: `std/rt/arena.go` uses a buffered channel of at most 128 arenas
(`arenaFree`; receive with `select`/`default`, send likewise, drop when
full). A channel also gives the happens before edge between the request
that released an arena and the one that reuses it. Cap what a kept object
retains (`arenaKeep`) so the free list cannot pin large buffers.
