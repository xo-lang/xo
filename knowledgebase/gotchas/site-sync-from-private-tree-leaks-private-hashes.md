# Syncing the website from a private checkout puts private commit hashes on the site

Status: Observed (2026-11-10, the 0.46.0 site, xolang.dev 1282aa9a5)

## Symptom

After the 0.46.0 release page push, content/docs/spec/std.md on xolang.dev said "the native one
since e0444461a", while the public v0.46.0 spec says e0444461a. The private hash does not resolve
on the public repository (Azure 1 noticed it while regenerating the stdlib anchors).

## Cause

The site's spec, std and diagnostics come from `scripts/sync-content.py REPO` (xolang.dev). The
0.46.0 sync was run on a worktree of the private repository at the v0.46.0 tag. publish.sh rewrites
commit hashes in the public tree (scrub rules in xo-publish-tools), so the private tree's text
differs from the public one wherever the spec names a commit.

## Fix

Sync from the public tree: `git clone --depth 1 --branch vX.Y.Z https://github.com/xo-lang/xo.git DIR`,
then `XO=<the released xo> scripts/sync-content.py DIR`. The released xo is the seed's bin/xo
(`seeds/X.Y.Z-<hash>/darwin-arm64.tar.gz`). The site's earlier release commits say "synced from the
public vX.Y.Z tree": take that literally.
