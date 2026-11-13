# A release built without its version bump says the old version

Status: Observed (2026-11-13)

## Symptom

A 0.51.0 dry run of `scripts/release.sh v0.51.0-dry` at main finished and
passed release-smoke, but its xo printed `xo 0.50.1` (SQ-PREBUILT named
it). Nothing in the build failed.

## Cause

xo's version is three literals in the source: compiler/cli/core/cli.xo
`VERSION`, compiler/cli/servers/servers.xo `XO_VERSION`, and std/VERSION.
The release commit bumps them (778b69ef0 did for 0.50.1), but the 0.51 cut
runbook had no such step, and release.sh took its version argument only to
name the archives.

## Fix

release.sh now refuses, before building anything, when its version (the
leading `v` and any `-suffix` dropped, so `v0.51.0-dry` wants 0.51.0) is
not all three literals, and refuses again before packaging when the built
xo's `xo version --json` reports another version. Bump all three in the
release commit. A trial at a commit that is not a release passes that
commit's own version (`v0.50.1-dry`).
