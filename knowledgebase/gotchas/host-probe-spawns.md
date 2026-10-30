# Host probes that start a process cost every build that asks

Status: Observed (2026-10-30)

**Symptom.** An HTTP server edit build on the arm64 offload host took about
96 ms; with XO_EMIT_JOBS preset it took about 78 (xo bench --ab, 0.874).
A logging wrapper on PATH counted 18 processes per build: `lscpu`,
`sysctl -n hw.memsize` and `sysctl -n hw.physicalcpu`, six of each.
On Windows a build ran one emit job.

**Cause.** compiler/xo split.xo's ncores and mem_gb asked `sysctl`, then
`lscpu` or `/proc/meminfo`, on every call, and emit_jobs called them for
each phase that sizes its tasks (six times in an edit build). On Linux the
`sysctl` keys do not exist, so each probe spawned a process that failed
before the one that answered. On Windows none of the commands exist from
cmd.exe, so ncores fell through to 1.

**Fix.** host_counts probes the physical cores once, at main's start and
only for the commands that compile, into XO_HOST_CORES. Linux reads sysfs
topology and Windows reads NUMBER_OF_PROCESSORS, with no process; macOS
runs one `sysctl`. mem_gb takes the runtime's `debug.mem_total()` first.
A host fact a build needs more than once is computed once per process,
and in process where the runtime or the file system can answer.
