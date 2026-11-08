# heavy.sh prints the command's output to its log, not to your stdout

Status: Observed (2026-11-05, twice: Xo Lang 4 and the lead; 2026-11-08, Xo Lang 4 again, through a pipe)

## Symptom

`HEAVY_CLASS=gate ~/xo-lead/agents/heavy.sh NAME CMD...` returns with
nothing on stdout, so a check that failed looks like it passed. Xo Lang 4
read an empty stdout from `go run ./scripts/xocheck compiler/xo` and
reported that xocheck missed an XO0605 the build caught; xocheck had
printed the error and exited 1. The lead's own `xo run` through heavy.sh
printed nothing either: its usage error and its program output were in the
log.

## Cause

heavy.sh (through heavyd) writes the command's stdout and stderr to
`~/xo-lead/agents/NAME-cmd.log`, between its `=== ... start` and
`=== ... exit N` lines. Only the exit status comes back to the caller.

## Fix

Read the result from the log, and trust the exit status, not stdout:

```
HEAVY_CLASS=gate ~/xo-lead/agents/heavy.sh NAME CMD...; rc=$?
sed -n '/ start /,$p' ~/xo-lead/agents/NAME-cmd.log | tail -40
```

A NAME reused across runs appends to the same log, so read from the last
`start` line. To keep the output in a file of your own, redirect inside the
command (`sh -c 'exec CMD > "$0" 2>&1' OUT`), as pushq.sh does.

Never pipe heavy.sh (`heavy.sh NAME CMD | tail; echo rc=$?`): `$?` is then
tail's status, 0, so a failed check reads as a pass twice over, once from
the empty output and once from the status. On 2026-11-08 a golden that
called `println` without `Os` (XO0601) passed `scripts/xocheck` that way,
and was reported as a checker gap until the log showed `exit 1`.

