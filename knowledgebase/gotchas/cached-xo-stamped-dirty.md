# A clean checkout's xo comes out stamped XO_DIRTY

Status: Observed (2026-10-24, DS-2's hollow fuzz run); fixed 2026-10-25.

Symptom: `go run ./scripts/xobuild` in a clean worktree at a commit
returns a bin/xo whose provenance stamps read another commit and a non
empty XO_DIRTY (`xo version -m bin/xo`). Any rule that refuses a dirty
xo for a measurement (agent-tools.md "Measuring": committed, non-dirty
xos) fails on a tree that has no uncommitted change.

Cause: the shared checkout cache (`<user cache>/xo/checkout/<key>`) is
keyed on the source key alone. A worktree with uncommitted changes whose
sources hash to the same key (the change was later committed unchanged)
built and stored its xo first, stamped with its commit and diff hash,
and the clean tree's xobuild reused that entry. The compiled sources are
identical; only the stamps are wrong.

Fix (2026-10-25, internal/xotool Ensure): the shared entry carries a `stamp`
file (the commit and the diff hash recordStamp finds) and is reused only by a
tree whose diff hash is the same (TestStampMatches); a tree with uncommitted
changes builds into a private `.tmp-PID-KEY` directory, removed after the
copy, and stores nothing in the shared cache. An entry with no stamp file (one
stored before this) matches no tree and is built again once. Cost: a tree
with any uncommitted tracked change (documentation included) builds its xo
instead of reusing the shared one when its bin/xo is missing or stale.
Before the fix, for a measurement or an identity check: delete bin/xo and
bin/xo.key and rebuild with a private `XO_CHECKOUT_CACHE` (deleted after),
then check `xo version -m bin/xo` shows the expected commit and an empty
XO_DIRTY.

Second bug of the same fix (2026-10-25, found by the seed cut's -short): two
test packages ran Ensure on a cold key at once. The first built the entry and
wrote its stamp; the second had read no stamp before, called
buildLockedForce(force), took the lock after the first, and `os.Remove(out)`
removed the freshly built xo while the first package's tests ran it: ten
cmd/xo tests failed with "no xo to run: open .../checkout/KEY/xo: no such
file". Fix: after taking the lock, a forced build returns when the entry's
stamp now matches the tree's; a rebuild no longer removes out (BuildXo's
rename replaces it). TestBuildLockedForceReusesConcurrentBuild holds the lock
while a forced build waits, builds the entry, and checks the waiter used it
(it failed before the fix with the build attempt).
