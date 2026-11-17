# Files put into /work/in on an offload host cannot be made executable

Status: Observed (2026-11-17, std/ui rooms Linux arena run on host 2)

## Symptom

A job script ran `chmod +x /work/in/arena-server-linux` on a binary
sent with `offload.sh --put FILE /work/in/FILE`, then ran it. The chmod
printed `Operation not permitted`, the run printed `Permission denied`,
and with nothing running the job used no CPU until offload.sh stopped it
after `OFF_IDLE_MIN` (15 minutes, rc 125).

## Cause

The put files belong to the host's transfer user, not the job's, and
keep the mode they arrived with (not executable). A job may read them
but not change their mode.

## Fix

Copy the files to a directory of the job's own and chmod the copies:

    d=$(mktemp -d /work/arena.XXXXXX)
    cp /work/in/arena-server-linux /work/in/arena-load-linux "$d"/
    chmod +x "$d"/*

Wrap each run in `timeout N` so a run that cannot start fails at once
instead of idling until OFF_IDLE_MIN.
