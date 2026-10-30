# A test that counts cache files also counts the detached prune's files

Status: Observed (2026-10-30)

**Symptom.** internal/arm64 TestMachoBinCache failed in a loaded main
check, "the second build added cache entries (3 to 4): not a hit", and
passed 3 of 3 at the same revision in a quiet gate.

**Cause.** A build that stores into the binary cache starts a detached
`xo dev prune-bins DIR` when a prune is due (compiler/xo cache.xo
prune_bins). The child writes prune-stamp, prune-sizes or prune-lock in
the same directory whenever it gets to run. Under load it ran after the
test's first count, so its file looked like a new entry from the second
build.

**Fix.** Count only the cache's own entries: skip `prune-*` and `*.lock`,
the files compiler/prune clean.xo treats as bookkeeping. Set
`XO_PRUNE_DETACH=off` in the builds so the prune runs inside the build.
The same applies to any test that counts files in a cache directory:
the backend cache's prune is detached too (bcache.xo
prune_backend_cache).
