# A Linux child's ru_maxrss counts the memory of the process it was forked from

Status: Observed (2026-11-02, Xo Lang 4: bench/perf/rwread on the second
linux/arm64 host; first written 2026-10-30 as Anticipated).

**Symptom.** TestTracebackIsolatedNoLeak (cmd/xo) failed in every
linux/arm64 host gate with a max RSS over 32 MB, while the program run
directly stayed flat at 6.8 MB.

**Cause.** `ProcessState.SysUsage().(*syscall.Rusage).Maxrss` of a child
the Go test started is, on Linux, the larger of the child's own peak and
the peak of the address space it had before the exec (the test binary's,
shared by the clone). A bound under the test binary's size cannot hold.

**Fix.** Poll `/proc/PID/status` `VmHWM` (the new address space's own peak)
while the child runs; keep `ru_maxrss` on macOS only.

**Observed 2026-11-02.** bench/perf/rwread/measure.py spawned each run
from Python (fork, then posix_spawn) and read wait4's ru_maxrss: 10.1 MB,
then 12.0 MB, for every run at 1 to 18 threads, Python's own peak. GNU
time and VmHWM gave 4.8 to 5.2 MB at 1 thread and 8.2 to 8.6 MB at 18.
posix_spawn does not avoid it (glibc's vfork shares the parent's address
space until the exec, and the exec records that space's peak). Fix used
there: start the run through /usr/bin/time (a small process) and read its
%M.
