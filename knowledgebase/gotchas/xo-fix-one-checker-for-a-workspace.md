# `xo fix ./...` on a large workspace used many GB: one checker for every member

Status: Observed (2026-11-20, task #537)

## Symptom

`xo fix ./...` at the root of the gcp SDK's tree (a workspace of 673
modules, 2,723 files, about 250 MB of source) peaked at 17 GB, then
20 GB in a second run (xw alert 94995), where `xo check ./...` on the
same tree stays near 1 GB.

## Cause

A checker holds about 50 bytes for every source byte it loads (55 MB
of source: 3.1 GB). `xo check` holds one workspace member's checker at
a time when a walk covers a workspace (decision 0513: `Ctx.per_module`,
check.xo load_each); `xo fix` called the same loader without that mode,
so every round of its fix loop checked every member in one checker.

## Fix

fix.xo's loop sets `per_module` on its loader context. On the gcp tree
`xo fix --dry-run ./...` peaks at 1.27 GB in 58 s. cmd/xo
TestFixWorkspaceMemory builds a workspace of twelve 1.4 MB members and
runs `xo fix ./...` under `XO_MAX_MEM=1500M`: over 3,000 MB before, under
700 MB after.

Two limits remain. `xo fix --vet=CODES` reads its fixes from the
session's one checker and keeps the single checker. And the split
needs packages outside the members (a root module, load_each's base):
a workspace with no root package is checked in one checker by `xo
check` and `xo fix` alike.

A first attempt batched the walked directories by source bytes inside
fix.xo. It was wrong: a package checked without its siblings resolves
their imports to the published copies in the module cache, not the
tree's, and reported and fixed other diagnostics.
