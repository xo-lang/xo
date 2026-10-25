# A test's env value placed before os.Environ() loses to an exported one

Status: Observed (2026-10-22)

Symptom: in the lead's full tier on main (ddd6879fa) five internal/arm64
cache tests failed with "no cache fe line" and "cold build: no `cache
chk off`": TestCacheStoreLog, TestFrontCacheStdCommentEdit,
TestFrontCacheMovedBodies, TestFrontCacheStdTestEdit,
TestParseListAfterRunCache. Their builds reused 0 parses even warm. They
pass when run by hand.

Cause: scripts/gate.sh exports `XO_BACKEND_CACHE=off` for every step,
and os/exec keeps the last value of a name that appears twice in
`cmd.Env`. TestCacheStoreLog built its env as its own settings followed by
`os.Environ()`, so the exported `off` won; the other four set
`XO_BACKEND_CACHE_DIR` but never `XO_BACKEND_CACHE`, so they inherited
`off`.

Fix: put a test's own settings after the environment,
`append(os.Environ(), "XO_BACKEND_CACHE=on", ...)`, and set every
variable the test depends on, not only the one it changes (bc352852d).
To reproduce a tier failure, run the test with the gate's exports
(`XO_BACKEND_CACHE=off XO_CLI=...`).

Again 2026-10-24: cmd/xo TestCacheStatsKeysHashes failed in tier 14 ("keys:
6 hashed writes and 0 hashed pack entries") for the same reason: it set
`XO_BACKEND_CACHE_DIR` but not `XO_BACKEND_CACHE`, so the gate's `off`
stored nothing (only the build dir's objects were written). It passed
alone. Fixed with `setenv(t, "XO_BACKEND_CACHE", "on")` (497e473dd). Any
test that needs the cache to store sets it explicitly.
