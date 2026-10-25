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
