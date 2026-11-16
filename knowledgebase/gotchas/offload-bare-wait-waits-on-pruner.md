# A bare `wait` in an offload job script never returns

Status: Observed (Xo Lang 3, 2026-11-15, task #77)

## Symptom

A job script for offload.sh that starts programs in the background and
then runs `wait` stops after 15 minutes with `under a second of CPU a
minute for 15 minutes: the job looks stuck and is stopped (OFF_IDLE_MIN)`.
The programs had finished; `/work/offload.runs/stopped.log` lists only
the job's bash and `offprune watch`, no process of the job's own. It
looked like the programs hung under parallel load (two runs of 8 and 4
copies of an HTTP test).

## Cause

The offload wrapper starts its disk pruner (`offprune watch PID`) as a
background child of the job's shell, so a bare `wait` (all children)
waits for the pruner too, which runs until the job ends.

## Fix

Wait for the job's own processes by the PIDs captured at launch:
`pids="$pids $!"` after each `&`, then `wait $pids`.
