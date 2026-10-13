# Trimming the shared Go cache breaks a concurrent build

Status: Observed (2026-10-08, internal/arm64 TestRun, once). Fixed: see
`go-cache-trim-races-builds.md` (Xo builds lock the cache) and
`go-commands-outside-build-race-trim.md` (every other go command does
too, 2026-10-13).

Symptom: a Go backend build in a test failed with `could not import os
(open .../Library/Caches/xo/gocache/a0/a0a8...-d: no such file or
directory)` and `could not import xoprog/rt`, reported as "go build ...
failed (code generator bug)". The same test passed on the next run with
no change.

Cause: Xo builds share one Go build cache (`<user cache>/xo/gocache`,
`gotchas/go-build-cache-growth.md`), and any xo process trims it by least
recent use at most every ten minutes. When several agents or test
binaries build at once, one process's trim can delete cache entries that
another process's `go build` has just looked up and is about to read.

Fix: none yet. Rerun the failing test; if it recurs, give concurrent
runs their own cache (`XO_GOCACHE=<dir>`), or make the trim skip entries
younger than a few minutes (`internal/build/gocache.go`). The message
names a code generator bug, which it is not; check for a missing file
under `gocache` before debugging the generator.
