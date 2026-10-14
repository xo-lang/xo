# Build directories under the user cache grew without a cap

Status: Observed (2026-10-10); fixed 2026-10-10.

**Symptom:** the disk reached 0 bytes free during a test run. `~/Library/Caches/xo` held 34 GB, 21 GB of it in `build/` (3,066 directories), although the Go cache (`gocache/`, 8 GiB cap) and the binary cache (`testbin/`, 6 GiB cap) were within their caps.

**Cause:** `buildDir` (internal/build/build.go) makes one stable directory per program, mode, and variant under `<user cache>/xo/build`, so Go's build cache is reused across builds. Nothing removed them, and every test copy of a program, every mode, and every worktree path hashes to a new one.

**Fix:** `trimBuildDirs` (internal/build/buildtrim.go) removes the least recently used directories once their total passes `XO_BUILD_DIR_MAX` bytes (default 4 GiB), down to three quarters of it, at most once a minute. `buildDir` refreshes a directory's mtime on each use, and a directory used within the last 2 hours is never removed. A directory set by `XO_BUILD_DIR` is never trimmed. Test: `TestTrimBuildDirs`.

Related: go-build-cache-growth.md.

Update (2026-10-14): this trim was deleted with the Go driver and the cache grew again; it is now pruned by age and size by selfhost/prune (caches-unpruned-after-go-deletion.md, decision 0223).
