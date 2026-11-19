# A container's /dev/shm is noexec, so staged --ab rounds fail

Status: Observed (2026-11-18, task #382)

## Symptom

`xo bench --ab` on host 1 (through ab-host.sh) failed in round 0 with
`fork/exec /dev/shm/xo-bench-NNN/a/prog-a: permission denied`, on every
pair, from 2282f94a7 (task #378, which stages A and B in /dev/shm).

## Cause

Docker mounts a container's /dev/shm as tmpfs with `noexec` (and 64 MB,
or `--shm-size`), and `--tmpfs DIR` mounts are `noexec` unless given
`exec`. Host 1's container (set up by hand) has
`shm /dev/shm tmpfs rw,nosuid,nodev,noexec,size=2097152k` and no other
tmpfs that holds a directory; without CAP_SYS_ADMIN it cannot remount.
The copy had mode 0o755: exec was refused by the mount. Docker's
masking tmpfs mounts (/proc/interrupts, /proc/kcore, /dev) look like rw
exec tmpfs in /proc/mounts but are not places to stage.

## Fix

xo bench picks the stage root from /proc/mounts (XO_BENCH_STAGE, /dev/shm,
/run/user/UID, another exec tmpfs, skipping /proc, /sys and /dev) and
exits 2 naming XO_BENCH_STAGE when none allows exec, never staging on
disk (the #347 bias). For a container that times programs, create it with
`--tmpfs /dev/shm:rw,exec,nosuid,size=8g` (or a dedicated exec tmpfs and
XO_BENCH_STAGE). Test: compiler/cli/bench bench_test.xo "--ab stages only
on a tmpfs that allows exec".
