# 0744: a child's wait parks on its exit event, not a polling backoff

Status: Accepted, implemented (owner 2026-11-22, xw ask #277, lead decision #278; task #705, xo-4c). Supersedes the "kqueue EVFILT_PROC or pidfd for waiting" alternative rejected in 0172.

## Context

std/native/llrt/proc.xo `wait_exit` (every `Child.wait`, `Child.wait_usage`
and `Proc.run`, std/pty's children too) calls wait4 with WNOHANG and,
while the child runs, sleeps 1 ms, doubling to 50 ms. A child that exits
in under 1 ms is reaped only after the first sleep. Decision 0172 chose
this over an exit event per OS ("a second mechanism per OS").

std/pty's perf twin (bench/perf/pty against github.com/creack/pty
v1.1.24) on linux/arm64 host 2 (stdproven perf, 30 rounds, main
af73bfcb2e): `spawn`, 300 children of `true` on a new pty, takes 527.2 ms
against Go's 72.5 ms, 7.308 of Go (1.76 ms against 0.24 ms per child);
darwin/arm64 1.241. A CPU profile of the Xo side spends 300 ms of 1405 ms
wall on CPU: the rest is the sleep. Go since 1.23 waits on a pidfd on
Linux.

## Decision

- Linux: `pidfd_open(pid, 0)` (syscall 434, Linux 5.3); the pidfd is
  registered with the poller for readable, the task parks until it is,
  then one wait4 WNOHANG reaps the child and the pidfd is closed. ENOSYS
  or EPERM (an older kernel, a seccomp filter) keeps today's backoff.
- darwin and FreeBSD: an `EVFILT_PROC` `NOTE_EXIT` one-shot filter on
  the poller's kqueue for the pid, the same park and reap; a child that
  exited before the filter was added (ESRCH) is reaped at once.
- Cancellation is unchanged: a canceled task's wait kills the child
  (SIGKILL) and then waits with no WNOHANG.
- Windows is unchanged (it already waits on the process handle).

## Consequences

- A short child is reaped when it exits: std/pty `spawn` and std/proc's
  run of short commands lose the 1 ms floor.
- Two more poller registration kinds (a pidfd fd, a proc filter); the
  backoff stays as the fallback, so it keeps its test.
- A time-budget guard: golden proc_wait_exit_event (a 70 ms child's
  spawn to wait, median of 5, under 100 ms; the backoff took 132 ms on
  darwin/arm64), and bench/perf/pty's `spawn` row against Go on host 2
  (decision 0726's 1.05).
- Measured (darwin/arm64, 2026-11-22): a 70 ms child 76 ms from spawn to
  wait (132 before); bench/perf/pty `spawn` (300 children of `true` on a
  pty) about 410 ms (about 540 before).

## Alternatives

- Start the backoff at 20 us (one line): still polling, and a long
  child costs more wakeups before the step reaches 50 ms. The lead
  declined it as an interim (2026-11-22): it would be replaced within
  days.
- A blocking wait4 on a thread per child (Go before 1.23): a thread per
  running child, and the pool's threads are not free to block.
- SIGCHLD with a self-pipe: process-wide signal state, and it does not
  say which child exited.
