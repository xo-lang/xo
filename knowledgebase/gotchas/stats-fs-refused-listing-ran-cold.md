# A wrapped Fs refused dir_names, and the build ran cold behind a passing test

Status: Observed (2026-10-24)

## Symptom

A build under `XO_CACHE_STATS=1` (or `=keys`) took 4 to 5 s for an edit that
takes 0.8 s without it, and wrote about 13,000 cache entries it had stored
already. `TestCacheStats` passed.

## Cause

`cache_get_all` (bcache.xo) listed a kind's shard directories with
`internal.dir_names(os.fs, ...)` and read `.ok() ?? []`. `dir_names` takes only
the unrestricted root Fs (`rt.fs_mode(f) != 0` is Denied). The stats wrapper
(`StatFs`, cachestats.xo) is a user Fs, so every listing was Denied, read as
"the cache holds no shard", and no key was opened: every lookup missed. The
test's cold-build assertion ("under 100 failed opens") passed because no open
was tried at all.

## Fix

A refusal (`FsErr.Denied`) lists the kind directory with `read_dir` (it holds
only the 256 shard directories); any other error is an empty cache.
`TestCacheStats` now also asserts the warm build read at least 1,000 entries
back (hello reads about 6,000), so a build that skips its lookups fails.

## Avoid

- An error mapped to an empty result (`.ok() ?? []`) turns a refusal into a
  silent miss. Map only NotFound to empty.
- A test of "few failures" needs a paired assertion that the thing was tried.
- A measurement through a wrapper Fs (stats, recorder) must be checked against
  the unwrapped build's time first.
