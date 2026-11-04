# heavy.sh's NAME-cmd.log appends: reading "the first match" reads an older run

Status: Observed (xo-lang-4, 2026-11-04).

## Symptom

Two `xo dev dump llvm` runs of the same program with two different xo
binaries, each through `heavy.sh l4ir ...` with stdout to /dev/null and
the function then cut out of `~/xo-lead/agents/l4ir-cmd.log` with
`awk '/define .../{p=1} p{print} p&&/^}/{exit}'`, gave identical IR. That
read as "the change makes no difference" (and, worse, "main already does
this"), and a working LLVM field-home prototype was reset as redundant.

## Cause

heavy.sh appends each job's output to `~/xo-lead/agents/NAME-cmd.log`;
it does not truncate it per run. An awk or grep that stops at the first
match reads the oldest run under that NAME, so every later run with the
same NAME printed the first binary's IR.

## Fix

- Write the job's output to its own file instead of reading the log:
  `XO_STD=... $X/bin/xo dev dump llvm DIR > $P/out-$v.ll` (no heavy.sh
  for a short read only command, or heavy.sh with a NAME unique to the
  run), or
- use a NAME per run (`l4ir-$v`), or read the last match (`tac` first,
  or the lines after the run's last `=== ... start` header).
- When two builds compare identical where a change was expected, check
  the comparison itself (which binary produced which text) before
  concluding the change does nothing.
