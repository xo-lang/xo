# A command's arguments never hold `DIR/...`

Status: Observed (Xo Lang 1, 2026-11-06)

## Symptom

`xo check` was to skip decision 0523's support check for a walk, tested as
`targets.any(fn(t) { t.ends_with("...") })` in compiler/cli/core/check.xo
run_check. It never matched: `xo check app/...` ran the support check, and
cmd/xo TestVouches, TestVouchEdges, TestVouchCheckCache and
TestCheckSupportPerMember failed with XO1201 on walks.

## Cause

compiler/cli/core/cli.xo strips the pattern before dispatching a command
that takes directories: `DIR/...` becomes `DIR` in the arguments and the
absolute DIR in `ctx.walk`. A command sees only plain directories.

## Fix

Ask `ctx.walk` (non-empty: the command line had a walk), never the
arguments' text.
