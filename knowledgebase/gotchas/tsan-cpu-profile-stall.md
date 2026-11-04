# A `--race` build can stall in a live CPU profile (debug_routes)

Status: Observed (Xo Lang 2, 2026-11-04). Fixed: SIGPROF is installed through ThreadSanitizer under `--race`.

## Symptom

TestGoldenTasks/debug_routes under ThreadSanitizer (`--race`) hangs
until go test's timeout: 3 of 4 full TSan rounds on main e622870f9 on
the linux/arm64 offload host 2, 2026-11-04. Run alone, outside go test,
on main d9dfdc95c with 8 runs in parallel plus 8 busy loops for load:

| build | XO_THREADS | runs | hangs |
|---|---|---|---|
| native | 2 | 200 | 0 |
| native | default | 200 | 0 |
| llvm | 2 | 200 | 0 |
| llvm | default | 200 | 0 |
| native --race | 2 | 800 | 1 |
| native --race | default | 800 | 3 |

Every hang stops after "unknown route: 404", in the program's
`/profile?seconds=1&hz=500` part (a live CPU profile through
ITIMER_PROF, with a second request refused while it runs). At the hang
every thread sleeps: 7 or 8 in futex_wait_queue (syscall 98), one in
hrtimer_nanosleep, one in epoll_wait; none runs. SIGQUIT writes no task
dump (TSan holds an async signal for its next interception point).

## Cause

On Linux the profiler installed its SIGPROF handler with a raw
`rt_sigaction` system call (std/native/llrt prof.xo `handler`, through
linux.sigaction), so ThreadSanitizer never saw it. TSan intercepts
`sigaction` to wrap each handler: a signal that lands while the thread
runs inside the sanitizer's own runtime (holding one of its internal
locks) is deferred to a safe point. The raw handler ran at once instead,
on whatever the interrupted thread held, and at 500 Hz during the
/profile request one sample eventually landed inside the runtime: the
threads then waited on each other's futexes and none ran. The plain
native and LLVM builds have no such runtime, so only `--race` stalled.

## Fix

Under ThreadSanitizer (sanhook.tsan(), its weak symbols present) the
handler is installed with the C library's `sigaction` (sys_c
weak_sigaction), which TSan's interceptor serves; other builds keep the
system call. With it, debug_routes' --race build ran 3200 times on host 2
(1600 at XO_THREADS=2 and 1600 at the default, 8 at a time beside 8 busy
loops) with no hang and no failure, where the same harness had 4 hangs in
1600 before (about 8 expected at that rate).
Sampling still works: a CPU burner built with `--race` and run with
XO_PROF=cpu XO_PROF_HZ=500 took 1247 samples, the plain native build
1248, over 2.5 s of CPU.

A deferred sample is taken at the safe point with the ucontext saved when
the signal arrived; the profiler's unwinder only follows frames inside
[sp, sp + PROF_SPAN) of that stack, so a stale context gives at worst a
wrong frame, never a wild read.

## Avoid

- Before the fix, a debug_routes TSan timeout was not a regression of
  the change under test: it hung on main too, and a TSan round of
  TestGoldenTasks for another change could skip it:
  `-skip 'TestGoldenTasks/debug_routes'`.
- To find the cause: rerun the race build on a host with gdb
  (`gdb -p PID -batch -ex 'thread apply all bt'` at the hang), or set
  `TSAN_OPTIONS=handle_sigprof=...`/`report_signal_unsafe` variants, and
  read which futex each thread waits on. The hunt script is in Xo Lang 2's
  notes (census/droutes.sh: native, llvm and --race builds of one
  golden, N runs P at a time, /proc thread states at a hang).
