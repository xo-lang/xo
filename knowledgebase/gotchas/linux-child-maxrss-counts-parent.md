# A Linux child's ru_maxrss counts the memory of the process it was forked from

Status: Anticipated (2026-10-30; confirm on the linux/arm64 host).

**Symptom.** TestTracebackIsolatedNoLeak (cmd/xo) failed in every
linux/arm64 host gate with a max RSS over 32 MB, while the program run
directly stayed flat at 6.8 MB.

**Cause.** `ProcessState.SysUsage().(*syscall.Rusage).Maxrss` of a child
the Go test started is, on Linux, the larger of the child's own peak and
the peak of the address space it had before the exec (the test binary's,
shared by the clone). A bound under the test binary's size cannot hold.

**Fix.** Poll `/proc/PID/status` `VmHWM` (the new address space's own peak)
while the child runs; keep `ru_maxrss` on macOS only.
