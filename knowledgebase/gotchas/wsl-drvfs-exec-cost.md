# A program run from /mnt/c on the WSL amd64 host pays about 6 ms of exec

Status: Observed (2026-11-08, Xo Lang 2)

## Symptom

release-vs-go.sh's linux/amd64 section read fib32 in 0.44.0 at 1.032 of
0.41.0 (13.9 against 13.5 ms, 9 rounds) and the 0.44.0 notes carried it as a
known issue. Every rerun from the WSL home was flat at about 7.9 ms: the
same range built with --release (0.993, n=11), the commit suspected
(2e032017b, 1.001, n=11), and the two release xos' Mac cross builds (1.002,
n=11).

## Cause

offload.sh --put lands files in the Windows home (xo-ci/off), which WSL
reads under /mnt/c/Users through drvfs, and release-vs-go-amd64.sh ran the
binaries from there. The same fib32 binary took 13.78 ms from /mnt/c and
7.86 ms from ext4 (`xo bench --ab`, 1.753, 1.67 to 1.867, n=11): a fixed
exec and page-in cost about as large as the work. It hides small changes
in short programs, skews any ratio against a binary of another size (Go's),
and the drvfs run's noise produced the 1.03 (the same pair from /mnt/c at
n=11 read 1.008, 0.998 to 1.032).

## Fix

release-vs-go-amd64.sh copies the three binaries to a temporary directory
under the WSL home before benching. Any timing on that host runs a program
from the Linux filesystem (`~/xo-ci/...`), never from /mnt/c.
