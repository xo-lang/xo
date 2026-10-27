# A key that hashes a cache entry's bytes misses what the replay set

Status: Observed (2026-10-27, Xo Lang 1)

## Symptom

With XO_CHAIN_KEYS=1 (decision 0382 stage 3), goldens built into one
shared back-end cache ran wrong: tls_coalesced_finish answered every post
with "tls: illegal parameter", other TLS and HTTP goldens and a debug
build's code differed from the cache-off build, and a library body moved
down three lines kept its old line table (TestFrontCacheMovedBodies).
XO_BACKEND_CACHE=verify passed on the single-program edits the gate tried.

## Cause

The cleanup cache's key hashed a replayed body's front end entry bytes in
place of the body. The replay (ssa fecache.xo fe_replay) does not give
the body those bytes alone: it shifts the lines by the declaration's line
(shift_lines), gives the body this build's name and target, numbers its
closures (fe_rename), its constants (fe_const_ids: rt.const_once's
number) and its spawn sites (fe_site_ids) as this build numbers them. Two
builds with one entry gave one key, so the second took the first's
cleaned-up body, with the first build's constant numbers and lines.

## Fix

The key also hashes what a replay sets (rccache.xo key_replayed: the
header, each value's line, position, aux and type, the debug rows), and
every stored entry carries the key of its body in full (key_func), which
verify compares, so an input a fast key leaves out fails verify even when
the output agrees. internal/arm64 TestChainKeysSharedCache builds nine
goldens into one cache, cold and warm, against the cache-off builds.
Whenever a key stands in for a body by where the body came from, list
everything the step that made the body read besides that source.
