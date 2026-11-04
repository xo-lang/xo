# The module check cache key misses what another module's xo.mod decides

Status: Observed (2026-11-04, Xo Lang 2, the v0.38.1 "not tested yet" list)

## Symptom

An app requires a version with `use go "os"`. `xo check` reports XO1316;
add the `vouch` line and it passes; remove the vouch again and `xo check`
still passes (also with a vouch for another Go package). With
`XO_CHK_CACHE=off` every step is right. `xo build` and `xo run` were
never affected (cmd/xo TestVouchBuildCache).

## Cause

The module check cache (decision 0351's addendum) keys an entry on the
module's path, its files and its interface closure
(compiler/check/modcache.xo mod_cache_load). The XO1316 of a dependency's
escape hatch depends on the consuming module's xo.mod (its vouch lines,
decision 0156), which is not in the dependency's key, so the entry stored
while the vouch existed replayed after it was removed.

## Fix

The key includes the consumer vouches that name the module (path,
version, kind, target, uses), and MOD_CACHE_VERSION went to v7 so entries
an older xo stored are not replayed. Test: cmd/xo TestVouchCheckCache.
Any new diagnostic of a module that depends on input outside the
module's own files (another xo.mod, the workspace, the environment) needs
that input in the key, or the module must not be cacheable.
