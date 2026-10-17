# Docker Desktop's VM is too small to build xo itself

Status: Observed (2026-10-15, xo-fa, scripts/docker-linux.sh).

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
