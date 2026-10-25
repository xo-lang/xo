# A clean checkout's xo comes out stamped XO_DIRTY

Status: Observed (2026-10-24, DS-2's hollow fuzz run).

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

Fix: for a measurement or an identity check, delete bin/xo and
bin/xo.key and rebuild with a private `XO_CHECKOUT_CACHE` (deleted
after), then check `xo version -m bin/xo` shows the expected commit and
an empty XO_DIRTY. Tool fix queued: xobuild should not reuse an entry
whose stamps differ from the tree's commit and dirty state (or should
not store a dirty tree's build in the shared cache).
