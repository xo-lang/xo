# Windows: a Toolhelp thread snapshot lists the whole system

Status: Observed 2026-11-21 (task #646, xo-4c).

## Symptom

`XO_PROF=cpu` on windows/amd64 took 2 to 6 samples for a run of 312 to
350 ms of CPU at the default 100 Hz (about 35 expected); `XO_PROF_HZ=1000`
gave about the same count. The darwin and linux profiles of the same
program were within 10% of CPU time.

## Cause

`CreateToolhelp32Snapshot(TH32CS_SNAPTHREAD, pid)` ignores the pid for
threads: it captures every thread on the system, and the caller filters
by `th32OwnerProcessID`. On the 32 core Windows offload host one
snapshot took about 11 ms and walking it about 90 ms (QPC counters in
win_tick), so one tick cost about 100 ms and the 10 ms timer fired late.
Suspending, capturing and unwinding the threads took under 1 ms.

## Fix

List the process's own threads with ntdll's `NtGetNextThread` (prof.xo
win_tick), keeping the snapshot only as a fallback. Golden
`internal/llvm/testdata/golden/native_prof_rate` reruns a fib child under
`XO_PROF=cpu` and requires samples times 10 ms within half to one and a
half of its CPU time (`ProcOutput.usage`); it fails before the fix (a ratio
of 0.06) and passes after it. Anything that walks threads per tick on
Windows should not use Toolhelp.
