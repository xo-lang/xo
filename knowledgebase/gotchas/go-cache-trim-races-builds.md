# Xo's Go cache trim deleted files a concurrent build was linking

Status: Observed (2026-10-08, `go test ./...`, `TestGoldenNoMono/test_property`)

Symptom: `link: cannot open file .../xo/gocache/a0/...-d: no such file or
directory`, reported as "go build failed (code generator bug)". Flaky:
`TestMCPTestTool` and `TestGoBindingSignatures` hit it too and passed on
rerun.

Cause: once the dedicated cache passed its 8 GiB cap, `trimGoCache`
removed files in mtime order. Go refreshes an entry's mtime only when it
is more than an hour old, so entries in use by a build running in another
test package or worktree could look old and be deleted between Go's cache
lookup and the link step.

Fix: never remove entries modified within the last 2 hours
(`keepRecent`), and restat each file just before removing it
(`internal/build/gocache.go`, test `TestTrimGoCacheKeepsRecent`). A
narrow window remains between Go's touch and the restat; a rebuild
recovers. If the cache still overflows because everything is recent,
it stays over the cap until entries age.

Update (2026-10-09): the keepRecent guard let the cache reach 94 GB.
Each generated program adds about 30 MB (every package re-instantiates the
runtime's generics; about a third is DWARF), and one test run writes far
more than the 8 GiB cap within two hours, so nothing was old enough to
remove. Fix: every Xo `go build` holds a shared flock on
`<gocache>/xo-cache.lock` (passing through `xo-gate.lock` so a waiting
trim is not starved); the trim takes it exclusively (without waiting,
or waiting once the cache is over twice its cap) and then removes any
entry, oldest first. Without the lock it falls back to keepRecent. The
trim runs at most once a minute. Tests:
`TestTrimGoCache{KeepsRecentDuringBuild,RemovesRecentWhenIdle,WaitsForBuild}`.
Older xo binaries (`.tools/bin/xo-0.6`) take no lock; a build of theirs
during a trim can still hit the original race and recovers on rerun.
