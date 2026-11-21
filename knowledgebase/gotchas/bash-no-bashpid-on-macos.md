# macOS bash 3.2 has no BASHPID, and $$ is the script's in every subshell

Status: Observed 2026-11-21 (task #582).

## Symptom

gate.sh's per-step peak sampler (`step_peak "$BASHPID" ...`) printed
`BASHPID: unbound variable` under `set -u` on the Mac and wrote no
`(peak)` rows; the same code works on the linux hosts' bash 5. The
failing checks were gate_failfast_test.sh's peakc and peaks cases.

## Cause

`/bin/bash` on macOS is bash 3.2, which has no `BASHPID` (added in bash
4). `$$` names the top-level script in every subshell and background
job, so it cannot name a step's subshell either.

## Fix

Get a subshell's own PID from a child: `sh -c 'echo $PPID'` run inside
it prints the subshell's PID. gate.sh's step_peak does this and takes
the step's shell as its parent (`ps -o ppid= -p PID`). Test on the Mac
(bash 3.2) as well as linux for any script that reads BASHPID; the other
gate.sh use (irmanifest, inside `$( )`) runs only on linux hosts.
