# Two A/B runs that shared one output directory measured garbage

Status: Observed (2026-10-28)

## Symptom

An interleaved link A/B (XO_STD_STORE_REPEAT timings, A and B xo binaries)
reported a B side whose total was twice the sum of its own steps (fib:
total 22.9 ms, prepare + resolve + finish 10.5 ms) and ratios of 1.5 to 2.1
for a change that removes work. A second A/B started meanwhile gave numbers
that matched neither run alone.

## Cause

Both scripts used the same fixed scratch directory for everything a round
builds: the copied program sources, both sides' back-end cache directories,
std's variants (std/<key>.store and .stdf from `xo dev stdcache`), and the
output binaries. Each script began with `rm -rf` of it. When the second
started, it deleted and rebuilt the first's caches and std variants
mid-run; the first's later rounds then built std's variant again inside
timed builds, linked against another side's store, or overwrote the binary
the other side was about to run. The heavy.sh queue does not prevent it: a
gate and a measure job run at once.

## Fix

Every measurement script writes under a directory of its own per run
(`O=$S/lab-$$`, removed by an EXIT trap), never a fixed shared name, and a
check that a side's total equals the sum of its steps flags a corrupted
run. Rerun any A/B that overlapped another job using the same directory.
