# A worktree's bin/xo can be older than its HEAD

Status: Observed (2026-11-08).

Symptom: a base-against-change comparison (instruction counts of
bench/perf/path) said a bounds check change made `path parts` 9.7% more
instructions, while the change's assembly for path only shrank. The
binaries differed in functions the change never touched
(`heapos.scavenge` was in one only).

Cause: the "base" xo was another worktree's `bin/xo`, taken because that
worktree's HEAD had the same compiler/ and std as the change's parent. Its
binary was built from an older commit (96a34435d) before the worktree
moved on; nothing rebuilds `bin/xo` until an xobuild or a Go test runs
there. A direct `bin/xo` call has no staleness check (Go tests refuse a
stale XO_CLI, the run's own message names the key and commit).

Fix: build the base at the exact parent in its own worktree (`git worktree
add --detach DIR PARENT`, then `go run ./scripts/xobuild` there) or use
ab-host.sh, which builds both revisions itself. The 9.7% was an artifact:
with an xo built at the parent, path parts, join and clean were flat.
