# A cache key stamped by a cache entry's modification time moves with the prune's touch

Status: Observed (2026-10-28, xo-lang-6, in the linux/arm64 Docker check
of the std records; the same on every platform).

## Symptom

An edit more than a minute after the previous one rebuilt std's
entries twice: `XO_CACHE_STATS=1` printed `cache cln 0/356 reused`,
`cache fe 0/...` and `cache stdx no entry` on two consecutive builds,
then hit again. Over six builds of one program the std records were
written under five keys.

## Cause

`cache_id` (compiler/xo/bcache.xo), the identity every backend cache
keys on, stamped the two std facts files (`std/<key>.stdf`) by path,
size and modification time, plus the content hash while the time was
within Go's 2 s settling window. The facts files are backend cache
entries, and the cache's `mark_used` sets an entry's modification time
when it is read more than `BACKEND_TOUCH` (1 minute) after its last
touch, for the prune's order. So a build a minute after the last one
touched the facts, its stamp changed (and, within the settle, carried
the hash), the next build's stamp changed again (no hash), and only the
third build's key matched a stored entry. Edits seconds apart never
showed it, which is why the Mac timelines were clean.

## Fix

A facts file that is a cache entry goes into the id by its name: the
key is `std_cache_key`'s, over xo's identity, the target, the variant
and every std source's size and time, so the name says what the file
holds (`facts_id_line`). A facts file that is not an entry (an explicit
`XO_SEPCOMP_FACTS`, a release's `lib/xo/stdstore`) keeps its stamp.

## Rule

Never put a cache entry's modification time into a key: the prune
owns that time. Key on the entry's name (content addressed) or its
content hash. Check `XO_CACHE_STATS=1` on a build more than a minute
after the previous one, not only on back-to-back edits.
