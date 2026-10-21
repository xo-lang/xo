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

## Addendum: a budget per run (2026-10-21, xo-fa)

A build that fell on an overdue prune after a busy period paid for all
of it: Lib 1's `-short` timed out when TestDevGoldensArm64's build of
one golden spent its 240 s unlinking back-end cache entries (about 2,000
of 3,333 stack samples in prune.remove; a rerun took 1 to 2 s). A run now
has a budget (Policy.budget, a time; Policy.max_remove, a count): past
it, it stops removing and sets the stamp due PARTIAL_PAUSE (a minute)
later, so the next build goes on and the same build's later stores do
not prune again. The time counts from the end of the walk, so a walk
longer than the budget still lets each run make progress. The driver's
prunes (build/, gocache/, the bin cache, the back-end cache) take 2 s,
`XO_CACHE_PRUNE_BUDGET` to change it (0 for none). A prune in a detached
child was the other option: it needs a process that outlives the build,
which std's Proc does not start, and the budget bounds the cost alone.
Tests: compiler/prune (a run past its budget, then the runs that go on;
the size cap under a budget) and internal/arm64 TestPruneBudget (an
overdue prune of 20,000 entries during a golden's build).

## Addendum: the back-end cache's cap is 16 GiB (owner, 2026-10-21)

A fully warm build of xo spent 85.9 s in insert_rc.sums (19.7 s user,
27.6 s sys), and a probe measured a warm build at 14.8 s against 9.1 s
with no cap (XO_CACHE_MAX_GB=100). The back-end cache held 11 GB in
332,535 files with about 8.8 GB used within two hours, above its 8 GiB
cap: each prune evicted entries the next builds stored again, and with
the burst rule (a prune once an eighth of the cap was stored) every few
builds paid for a prune unlinking tens of thousands of files. The
default cap is now 16 GiB (bcache.xo BACKEND_CAP_GB), above that working
set; XO_BACKEND_CACHE_MAX and XO_CACHE_MAX_GB still override it. With the
run budget above (2 s of removals a run) and a pack compaction counting
only what it adds, a prune no longer dominates a build.

Addendum (2026-10-21, xo-fa): the back-end cache is pruned a slice at a
time, off the build. A whole walk of a busy cache took 44 to 48 s (about
500,000 files, mostly lstat); a build that fell on it stalled, and a
build killed during it left a fresh stamp that blocked prunes for an
hour, so the cache reached 62.5 GB under its 16 GiB cap. Now a due
prune runs in a detached `xo dev prune-backend` child. Each run walks
the shards (`fn/ab`, `packs/ab`) after a cursor for 300 ms
(prune.Policy.walk_budget, slice_shards), keeps per-shard sizes in a
ledger, and evicts each walked shard's least recently used entries down
to its share of the cap (shards are uniform: keys are SHA-256). A run
holds a lock that goes stale after 10 minutes; the stamp is written when
a run ends; a log line records each run. Pack slots move under
packs/<xx>/. Alternatives: a size ledger updated on every store (an
append per store from many processes, and every removal by another
process to account for), or keeping the in-build whole walk with a
longer interval (the build that falls on it still stalls).
