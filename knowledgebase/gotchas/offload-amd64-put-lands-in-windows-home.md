# offload-amd64.env's --put lands in the Windows home, not the job's HOME

Status: Observed (2026-11-17, Xo Lang 3)

## Symptom

A file sent with `OFFLOAD_ENV=offload-amd64.env offload.sh --put FILE
xo-ci/off/NAME` was not found by a job that ran `cd ~/xo-ci/off` (cp:
cannot stat). Twice a 20 minute sampling job ran with no program.

## Cause

offload-amd64.env is WSL2 on the Windows host. `--put REMOTE` is relative
to the Windows user's home, which WSL mounts at /mnt/c/Users/xo, while a
job's shell starts with HOME=/home/xo (and its working directory at
/mnt/c/Users/xo). `~/xo-ci/off` in the job is /home/xo/xo-ci/off, not where
the file went. offload.sh's header says so ("--put REMOTE is then relative
to the" Windows home), and amd64-run.sh fetches its bundle by that path.

## Fix

In the job, use the absolute path /mnt/c/Users/xo/xo-ci/off/NAME (or `cd`
there and fail at once when it is missing: `cd DIR || exit 3`), and check
the file exists before a long loop starts.
