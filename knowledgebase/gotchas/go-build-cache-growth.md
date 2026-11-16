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

Update (2026-10-14): this trim was deleted with the Go driver and the cache grew again; it is now pruned by age and size by compiler/prune (caches-unpruned-after-go-deletion.md, decision 0223).

Update (2026-11-16): Go's own default cache (`~/Library/Caches/go-build`,
used by `go test`, scripts and twins, not by Xo builds) reached 712 GB on
the Mac and filled the disk; Docker's VM went read only with it, taking
offload hosts 3 and 4 down until Docker restarted. Go trims that cache
only by 5 days of age. Guard: scripts/lead/diskguard.sh, run by
mainwatch.sh at every wake and every 30 minutes of a wait, deletes its
files by age (7 days, 3, 1, then 6 hours, stopping once 200 GB are free)
and records an xw `disk.low` event under 100 GB. It never deletes a file
touched in the last 6 hours: Go refreshes an entry's time when a build
uses it (at most hourly), so a running build keeps its entries
(gocache-trim-races-concurrent-build.md).
