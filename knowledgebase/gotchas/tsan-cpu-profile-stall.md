# A `--race` build can stall in a live CPU profile (debug_routes)

Status: Observed (Xo Lang 2, 2026-11-04). Root cause open.

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

Not found. Only `--race` builds stall, so it is ThreadSanitizer's
signal handling meeting the profiler's SIGPROF (std/native/llrt/prof.xo
on_signal) and a futex wait: a wakeup lost while TSan defers a
signal is the leading guess. The plain native and LLVM builds ran
800 times without a hang, so a release is not shipping this hang.

## Avoid

- Do not read a debug_routes TSan timeout as a regression of the change
  under test: it hangs on main too. A TSan round of TestGoldenTasks for
  another change can skip it:
  `-skip 'TestGoldenTasks/debug_routes'`.
- To find the cause: rerun the race build on a host with gdb
  (`gdb -p PID -batch -ex 'thread apply all bt'` at the hang), or set
  `TSAN_OPTIONS=handle_sigprof=...`/`report_signal_unsafe` variants, and
  read which futex each thread waits on. The hunt script is in Xo Lang 2's
  notes (census/droutes.sh: native, llvm and --race builds of one
  golden, N runs P at a time, /proc thread states at a hang).
