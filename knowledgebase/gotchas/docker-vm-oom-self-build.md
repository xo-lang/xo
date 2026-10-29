# Docker Desktop's VM is too small to build xo itself

Status: Observed (2026-10-15, xo-fa, scripts/docker-linux.sh; 2026-10-29 under
box64 with the measured peak, xo-lang-6).

## Symptom

`xo build compiler/xo` inside a linux/arm64 container (seed 0.14.0)
prints `Killed` after the checker's warnings; no binary.

## Cause

Docker Desktop's Linux VM has 8 GB on this Mac (`docker info`: MemTotal
8217751552), and a self build of xo needs more than that with its
default parallelism.

## Fix

Cross build the Linux xo on the Mac with the Go backend, as
scripts/seed-cut.sh does:

    bin/xo build --backend=go --targets=linux/arm64 --std=std -o OUT/bin/xo compiler/xo

and copy std/ next to it (OUT/std). Or raise the VM's memory in Docker
Desktop's settings (the owner's machine: ask first).
## Also under Wine (box64), and the number (2026-10-29, xo-lang-6)

`wine s1.exe build --backend=amd64 -o s2.exe compiler/xo` (the Windows
self-build fixed point: s1 cross built on the Mac with the LLVM backend and
winxc, s2 under box64 in scripts/docker-wine.sh's container) printed
`Killed` after the checker's warning line, about 20 minutes in, no s2.exe.
With `XO_THREADS=2 XO_MAX_MEM=6G` in the wine command's environment (Wine
hands the Unix environment to the program) xo's own budget fired instead,
after 8 minutes: `memory budget exceeded: 6155 MB mapped ... while:
building, past phase dce`. The peak is the program's data, not the worker
count: the same cross build on the Mac (`/usr/bin/time -l`, cache off)
peaks at 8.25 GB with XO_THREADS=1 and 8.21 GB with 2, so no thread
setting fits it in the VM's 8 GB (7.3 GB free inside). The default budget
(decision 0349) is off on Windows, where the physical memory is not read,
so without XO_MAX_MEM the kernel's kill is the only report. Fix: the VM's
memory raised to 12 GB or more (the owner's setting), or the fixed point
on the Windows CI runner.
