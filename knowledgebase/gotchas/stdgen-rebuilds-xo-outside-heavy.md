# stdgen and xocheck rebuild bin/xo outside heavy.sh

Status: Observed (xo-lang-6, 2026-10-30)

## Symptom

`go run ./scripts/stdgen` (after a std/debug.xo edit) printed
`xotool: building xo (compiler/xo of ...) with the checkout's earlier
bin/xo, a few minutes` and rebuilt the worktree's xo for 92 s, several
GB, directly in the session, outside any heavy.sh slot or gate.

## Cause

stdgen, like `go run ./scripts/xocheck`, needs the checkout's xo
(internal/xotool), and xotool rebuilds bin/xo whenever compiler/ or a
std module it uses changed since the last build. A std edit changes the
source key, so the first Go tool to ask for xo builds it.

## Fix

Run stdgen and xocheck through a gate (`HEAVY_CLASS=gate
~/xo-lead/agents/heavy.sh NAME script`) when the tree has compiler or
std changes, or set `XO_CLI` to an already built xo of that tree first.
A tree with no such change (docs only) runs them directly.
