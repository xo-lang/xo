# Worker count ignored the CPU affinity mask on Linux

Status: Observed (2026-10-21)

## Symptom

The CRUD server pinned with `taskset -c 0-2` in Docker (12 CPUs) ran as
if it had 12 CPUs: up to 12 workers on 3 cores. Pinned 1, 3, and 6 core
measurements (decision 0324's Linux runs, the negative scaling table of
2026-10-21) compared oversubscribed pools, not pools of 1, 3, and 6.

## Cause

poolsetup.xo cpu_count read /sys/devices/system/cpu/online (the CPUs the
kernel has online), not the CPUs the process may run on. taskset and a
container's cpuset (`docker run --cpuset-cpus`, Kubernetes static CPU
policy) narrow the affinity mask, which only sched_getaffinity reports.
Go sizes GOMAXPROCS from sched_getaffinity.

## Fix

cpu_count counts the bits of sched_getaffinity's mask (128 bytes, 1024
CPUs) and falls back to the online file when the call fails. XO_THREADS
still overrides. A CPU quota (cgroup cpu.max) is not read; Go 1.25 reads
it, Xo does not yet.

## How to avoid

When measuring with pinned cores, check the worker count (`grep Threads
/proc/PID/status`), or set XO_THREADS to the pinned count.
