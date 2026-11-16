# 0675: C calls and the scheduler: no preemption signal into C, and a worker blocked in C is handed off

Status: Proposed (Xo Lang 1, 2026-11-16; task #94)

## Context

aws-43 found two scheduler faults through std/sqlite's stress run
(2026-11-16, repro ~/xo-lead/notes/azure-1/csleep, task #94):

1. **SIGURG reaches workers blocked in C.** Asynchronous preemption
   (decision 0318) has the monitor send SIGURG to a worker that ran its
   fiber a slice (SLICE_NS, 10 ms) while work waits. The handler declines
   at a PC outside the program's code, but the signal has already
   interrupted the C call: nanosleep returns EINTR. In the repro (8 tasks
   calling a C nanosleep(100 ms) 10 times beside 8 spinning tasks) every
   sleep returned early, 80 of 80, at XO_THREADS=4 (0 of 80 at 30).
   SQLite's busy handler sleeps with nanosleep and counts the whole delay,
   so a 10 s `_busy_timeout` ran out in milliseconds.
2. **A worker blocked in C is not replaced.** XO_THREADS=N bounds the
   threads running Xo work (decision 0640, Go's P), and a worker inside a
   C call still counts against it. C code that waits for another task
   (SQLite's lock held by a connection whose task is queued) can starve
   that task: SQLite at 64 clients on 2 workers stalled.

Go's design (runtime/cgocall.go, proc.go): a cgo call enters a syscall
state (entersyscall: the M's status and its P's syscalltick); sysmon
never preempts a thread in a syscall or cgo call, and retakes the P of
one that has been in it for a sysmon tick while work waits (handoffp),
so another M runs Go code; on return (exitsyscall) the M takes a P back
or parks.

Every `use c` call (decision 0116) goes through its generated C adapter
(compiler/cli/bind gen.xo, `xo_<prefix><name>`), which already calls the
runtime (`xo_x_cffi_str`). The pool's workers are started on demand and
their count is bounded by worker_cap (decisions 0377 and 0640); the
monitor (mon_scan) ticks while work waits.

## Decision

1. **A worker marks its C calls.** Each adapter brackets the C call with
   two runtime exports, `xo_x_c_enter()` and `xo_x_c_leave()`. They add
   one to a word of the calling thread's worker record, W_CSEQ (odd while
   in C), with a release store: one thread local load and one store each,
   no shared atomic (a counter on the pool would bounce one cache line
   between every worker making C calls; SQLite makes millions). A thread
   that is no worker (a C thread calling back, a root outside its loop)
   does nothing.
2. **No preemption signal into C.** mon_scan does not ask a worker whose
   W_CSEQ is odd to yield and sends it no SIGURG (an ask already out is
   withdrawn as for a blocked worker). The handler's decline at a C PC
   stays for calls the runtime makes itself.
3. **A worker blocked in C is handed off.** mon_scan records each
   worker's W_CSEQ; a worker whose odd W_CSEQ is unchanged since the
   previous tick (in the same C call for at least one tick) while work
   waits and no worker is idle no longer counts against worker_cap: the
   monitor raises the pool's allowance P_CEXTRA to the number of such
   workers and calls wakep, which starts or wakes one more worker. When
   the call returns the worker goes on with its fiber (pinned to its
   thread, as now); the allowance falls at the next tick, and a worker
   past the cap parks at its next look for work (go_idle), as one above
   a lowered cap does today (decision 0377). Threads running Xo code can
   exceed N for up to a tick, as Go's Ms can while one returns from a
   syscall without a P.

Tests that fail first: the repro as a test program (testdata/rt) whose
C nanosleeps must not return early with spinning tasks beside them
(item 2), and a program where a task blocked in a C wait for another
task's progress finishes at XO_THREADS=1 (item 3).

## Consequences

- C calls cost two runtime calls more (a thread local load and a store
  each); measured on bench/perf/sqlite before and after (the twin's rows,
  host 1).
- SQLite's busy timeout and any C code that sleeps or blocks behave as
  under Go: a sleep is not cut short, and a blocked C call does not take
  a worker from the program's tasks.
- The monitor runs while a worker is in C and work waits (it ticks while
  work waits today).
- Windows adapters (clang's Win64 code through the x86win thunk) call the
  two exports as they call xo_x_cffi_str.

## Alternatives

- **SA_RESTART or retrying in the adapter:** nanosleep is never restarted
  (POSIX), and a retry cannot know the remaining time of an arbitrary C
  call.
- **Block SIGURG around C calls (pthread_sigmask):** two system calls per
  C call, and it leaves item 3.
- **A shared in-C counter on the pool:** one atomic add per C call on a
  word every worker writes.
- **Treat every C call as blocking and hand off at once:** a thread
  switch per short C call (sqlite3_column_int); Go hands off only after a
  sysmon tick for the same reason.
