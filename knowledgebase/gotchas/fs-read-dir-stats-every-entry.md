# fs.read_dir stats every entry: a listing of a cache shard is not cheap

Status: Observed (2026-10-23)

## Symptom

Builds against a large backend cache crawl. Main's check at 19d14c33c
timed out 37 of 87 windows/amd64 cross golden builds at 120 s (`xo dev
build-each`), and internal/arm64 and internal/llvm `-short` hit go
test's 10 minute limit. A stack sample of a hung build
(`~/Library/Caches/xo/dev/goldens/bin-arm64/hang-arith.txt`) had the main
fiber in build_native → lower → fe_cached_build → cache_get_packed →
cache_get_all → prelude.fs_read_dir → sysio.fs_stat → darwin.stat in
3,128 of 3,399 samples.

## Cause

`Fs.read_dir` returns a `FileInfo` per entry, so it stats every entry of
the directory. 284b87da9 made cache_get_all (bcache.xo) list each backend
cache shard directory it needed 8 or more keys from, to skip failed
opens on a cold build. On the shared user cache (19 GB backend, thousands
of files per shard) every lookup stat'ed whole shards, far more work than
the failed opens it saved. It was measured only on small private caches
(`XO_BACKEND_CACHE_DIR` in a scratch directory), where a shard holds a few
files and a listing costs nothing: cold builds there went from 13.6 s to
7.8 s.

## Fix

Reverted (712928e22; the windows/amd64 cross builds of arith, the hung
golden, and four others took 2 to 4 s again against the real cache). To
bring the miss reduction back: list names only (a names-only listing, no
stat per entry), and measure on the real shared user cache, with its
size stated, before pushing, never only on a small private one. A cache
change's measurement names the cache it ran against.
