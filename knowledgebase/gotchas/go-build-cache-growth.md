# Xo builds filled Go's build cache (189 GB)

Status: Observed (2026-10-08 overnight). Resolved the same day.

Every generated program, and every test copy of one (goldens, `-race`,
`--mono` on and off, benchmark runs), adds about 15 MB of compiled packages
to Go's build cache. Go trims its cache only by age (5 days), so a night of
agent builds filled `~/Library/Caches/go-build` to 189 GB and the disk to 100
percent, which made tests fail with disk full errors.

Fix: `internal/build/gocache.go` gives Xo builds their own cache
(`$XO_GOCACHE`, default `<user cache>/xo/gocache`) and trims it to three
quarters of `XO_GOCACHE_MAX` (default 8 GiB) by least recent use, at most
every ten minutes. `internal/gobind` uses the same directory. To reclaim
space by hand: `rm -rf ~/Library/Caches/xo/gocache` (and `go clean -cache`
for Go's default cache).

Update (2026-10-09): a generated program now adds 3 to 9 MB, not 15 to
34 MB (`go-generic-method-sets-per-package.md`).
