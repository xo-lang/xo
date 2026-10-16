# 0223: Age and size pruning of xo's caches

Status: Accepted (owner request, 2026-10-14; implemented by the prune
agent the same day).

## Context

On 2026-10-14 the Mac's `<user cache>/xo` grew about 12 GB in an hour
while five agents built: `build/` reached 26 GB, `gocache/` 22 GB,
`checkout/` 5 GB, `testbin/` 5 GB. The Go driver had trimmed `build/`
(trimBuildDirs, 4 GiB) and `gocache/` (trimGoCache, 8 GiB) by size;
the deletion of the Go compiler (decision 0215) removed both, and xo1's
build_dir left trimming "to the Go driver"
(gotchas/caches-unpruned-after-go-deletion.md). Nothing pruned
`checkout/<key>`. The owner had them cleared and asked for automatic
pruning by age and size, like Go's own cache trimming but tuned for heavy
use.

## Decision

One routine, `selfhost/prune` (`prune.run`, `prune.mark_used`), and one
stamp format for every cache xo writes:

- An entry's last use is its modification time. A reader marks an entry
  used with `mark_used`, which sets the time only when it is over an hour
  old (one stat per hit otherwise, as Go's cache does). Writers store
  under a temporary name and rename.
- A cache's last prune is the time of `DIR/prune-stamp`. `run` does
  nothing while the stamp is younger than the policy's interval.
- `run` removes entries unused for longer than `max_age`, then the least
  recently used while the cache is over `max_bytes`, until it is under.
- An entry used within two hours is never removed, and each entry is
  restated before removal. Two hours, not one: `mark_used` and Go leave
  a time up to an hour old on an entry in use, so the margin covers a
  build of up to an hour after a refresh.
- An entry is either each top level directory (`build/`) or each file
  below the top level (`gocache/`, the bin cache, the back-end cache);
  top level files (stamps, locks, Go's README and trim.txt) never are.

Limits: `XO_CACHE_MAX_AGE` (a Go duration, default 24h, 0 off) and
`XO_CACHE_MAX_GB` (GiB, default 20, 0 off) for `build/` and `gocache/`;
the older byte caps `XO_BUILD_DIR_MAX`, `XO_GOCACHE_MAX`, and
`XO_BIN_CACHE_MAX` (default 6 GiB for the bin cache, pruned at most once
a minute as before) take precedence when set. `build/` is pruned in
build_dir only when `XO_BUILD_DIR` is unset (never a directory the user
chose); `gocache/` after each go build.

`gocache/` is pruned per file, not with `go clean -cache`: Go refreshes
an entry's time when it uses one older than an hour and treats a missing
file as a miss, so removing files unused for two hours cannot break a go
command in flight, and needs no cache lock (the old size trim removed
recent entries under the exclusive lock of gotchas/go-commands-outside-
build-race-trim.md; this one never removes recent entries).

internal/xotool prunes `checkout/<key>` (Go, the same stamp format,
prune.go): at most hourly, keys beyond the newest `XO_CHECKOUT_KEEP`
(default 4) unused for longer than `XO_CACHE_MAX_AGE` go; Ensure marks
its key used and never removes it.

## Consequences

- A cache can exceed its cap by what was used in the last two hours,
  and between prunes (an hour): a cap is a bound on idle data, not a
  hard limit.
- A prune walks the cache: hourly for `build/` and `gocache/`.
- The back-end cache of decision 0220's addendum (bcache.xo, in work on
  2026-10-14) uses the same stamp file and entry times, so it can call
  `prune.run(os.fs, os.clock, dir, prune.Policy{...})` with its own
  limits instead of its own loop.
- `rtobj/` keeps its size trim (trim_cache_dir, 512 MiB); `dev/`,
  `mbedtls/`, `seeds/`, and `seed/` are not pruned.

## Addendum (2026-10-16): burst prunes and the back-end cache's keep

On 2026-10-16 parallel builds grew `<user cache>/xo/backend` from about
2 GiB to 19 GiB in 15 minutes against its 8 GiB cap: 556k of its 659k
entries were under two hours old, so the hourly prune could remove none
(gotchas/backend-cache-burst-past-cap.md). Two changes (lead request,
2026-10-16):

- `Policy.burst`: when over zero, a run within `every` still prunes once
  the bytes stored since the last prune reach it. Writers record their
  stores with `prune.note_stored(fs, dir, n)` in `DIR/prune-stored`
  (decimal text, removed by each prune; two writers at once can lose a
  count, which the next store or the hourly prune covers).
- The back-end cache (selfhost/xo bcache.xo) uses keep 2 minutes, touch
  1 minute, and burst an eighth of its cap. Its entries are read whole,
  so removing a young one costs a later hit, never a build. The cache
  now stays within its cap plus an eighth, plus what was stored in the
  last two minutes.

## Alternatives

- `go clean -cache` when over the cap: simple, but it drops the whole
  cache, including the runtime's packages every build needs next.
- Size caps alone (the Go driver's trims): a cap set for heavy days keeps
  stale data forever on light ones; the age limit returns it.
- Access times: unreliable (noatime, relatime), hence explicit times.
