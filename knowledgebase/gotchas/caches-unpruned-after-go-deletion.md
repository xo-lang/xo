# The cache trims went away with the Go driver

Status: Observed (2026-10-14); fixed 2026-10-14 (decision 0223).

**Symptom:** `<user cache>/xo` grew about 12 GB in an hour while five
agents built on the Mac: `build/` 26 GB and `gocache/` 22 GB, both over
the caps the toolchain documented (4 GiB and 8 GiB,
gotchas/build-dir-growth.md, go-build-cache-growth.md).

**Cause:** the trims lived in the Go driver (internal/build buildtrim.go
trimBuildDirs, gocache.go trimGoCache), deleted with the Go compiler
(decision 0215). xo1 computes the same directories (selfhost/xo cache.xo
build_dir, gobackend.xo go_cache_dir) but its build_dir said "trimming is
left to the Go driver", and nothing called a trim of the Go cache. Only
the bin cache (`testbin/`, 6 GiB) and `rtobj/` (512 MiB) were still
trimmed, by xo1's own trim_cache_dir. A port that keeps keys and paths
identical still drops behavior that ran beside them.

**Fix:** selfhost/prune prunes `build/` (in build_dir), `gocache/` (after
each go build), and the bin cache by age and size; internal/xotool
prunes `checkout/<key>` (decision 0223, toolchain.md "Cache pruning").
When porting or deleting a component, list its side duties (trims,
locks, stamps) and check each has a new owner.
