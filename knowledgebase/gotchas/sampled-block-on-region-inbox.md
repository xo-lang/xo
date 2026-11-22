# A heap profile's sampled block on the heap region's inbox

Status: Observed (2026-11-21, xo-4c found it, Xo Lang 1 fixed it, task #658).

Symptom: `XO_PROF=heap XO_PROF_HEAP_RATE=1 XO_PROF_HEAP_AT=fe.build xo
build` of a large program wrote its profile and then died with `fatal
error: SIGSEGV` in `heapos.ra_cover` under `heapos.region_rel_cover`
(`p = rt.mem_load(p, 0, 8)`: a free list link of 1 or 3), one run in
three to five on darwin/arm64 and linux/arm64. 0.50.1 was right; the
region came in 0.52.0 (decision 0633).

Cause: the heap profile marks a sampled block of a size class by
storing minus the class in its header, and alloc.xo's frees send a
negative header to `heap_big_release`, which makes the profile forget
the block, restores the class and frees it again. `free_mt` tested "is
this a block of the heap region" before that, so a sampled region block
went onto the record's inbox as it was. The region's release drains the
inboxes onto the region's lists by the class in the header: with a
negative class the list's address is below the record's lists, and the
push stored a pointer and added 1 to a count in words that belong to
other things. The damage was done on every such run; whether the walk
then met a bad link was chance.

Fix: `free_mt`'s region branch sends a negative header through
`heap_big_release` first, as its other branches do.

How to avoid: a new fast path in a free that returns early must come
after the header's sign test, or repeat it. Any code that reads a
block's class from its header (the inbox drain) relies on that.
cmd/xo TestHeapProfileRegionBuild runs the build ten times.
