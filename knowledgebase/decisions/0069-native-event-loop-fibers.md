# 0069: Native tasks run on fibers; an event loop for timers, sockets, and signals

Status: Accepted (2026-10-08). Supersedes point 1 of decision 0068 (a
blocked task holds a thread) and narrows its point 7 (deadlock detection).
Implemented in `std/llrt/xo_rt.c` ("Fibers", "The event loop", "Clock",
"Network", "Signals and two phase shutdown"), `internal/ssa/build_io.go`,
`internal/ssa/build_conc.go` (`with_timeout`, `after`, `stopping`).
Tests: `std/llrt/testdata/evloop.c` (TestRuntimeEventLoop),
`internal/llvm/testdata/golden/clock_timers`, `net_echo`,
`internal/llvm/signal_test.go` (TestNativeSignal*).

## Context
Decision 0068 ran each task to completion on a worker thread; a task
that waited held its thread (64 MiB of reserved stack, at most 4096
workers). The native HTTP server needs the clock, sockets, and signals,
and a server waits on thousands of idle connections: one thread per
waiting task does not scale, and a thread blocked in a socket call cannot
be canceled. Tasks that wait must give their worker back. Two ways:
continuations (the compiler splits every function that may wait into a
state machine, as Rust's async and C#'s await do) or stackful tasks
(each task has its own stack and the runtime switches stacks, as Go
does).

## Decision
1. **Stackful tasks (fibers).** Every task a worker starts runs on a
   fiber: a stack of its own (64 MiB reserved with a guard page below,
   `XO_TASK_STACK_MB` overrides, a cache of 64 stacks) and a hand written
   switch of the callee saved registers (arm64 and x86_64, in the C file
   as top level assembly). Every wait of the runtime goes through one
   function (`park_wait_n`): on a fiber it marks the fiber parking,
   switches to its worker's scheduler, and the worker runs other work. A
   signal of the park queues the fiber again (its pointer with the low
   bit set, in the same deques as new tasks) and any worker resumes it.
   Parking is a handshake on the fiber's state (running, parking,
   parked, woken), so a wake up is never lost and a fiber never runs on
   two workers. The main thread is not a fiber: it blocks on its
   condition variable as before. So every wait parks, not only I/O and
   timers: channels, locks, awaits, and scope joins too. Chosen over
   continuations because it changes no generated code and no calling
   convention: the SSA, both backends, the unwinding checks, and
   reference counting stay as they are. Continuations would split every
   function that may wait, including through function values and generic
   instances, in both backends.
2. **Thread locals through functions that are never inlined.** A fiber
   may continue on another thread after it parks, and compilers treat a
   thread local's address as constant within a function (also across a
   call, and after link time inlining into generated code), so the
   runtime reads and writes `cur`, the current fiber, and the worker
   only through `noinline` accessors, and reads errno only in `noinline`
   system call wrappers that return -errno.
3. **One poller thread**, started at program start: kqueue on Darwin,
   epoll on Linux, edge triggered, with a self pipe for wake ups and
   signals. Timers are a binary heap by monotonic deadline; a timer
   signals a park (sleep) or runs a function on the poller thread (select
   `after`, `with_timeout`'s deadline, the grace period). A socket has a
   descriptor with the waiting park of each direction or a readiness that
   came while nobody waited; it closes its socket only when no operation
   runs, and the poller frees it between two waits.
4. **Clock**: `now` (wall clock ns), `sleep` (a cancellation point that
   unwinds, parked on a timer), `since`; `after d` in `select` (the arm
   wins when no other proceeds within d); `task.with_timeout` runs the
   closure as the body of a scope whose deadline timer cancels it; when
   the closure unwinds because of the deadline (not a cancellation from
   outside) the result is `Err(TimedOut)`.
5. **Deadlock detection (0068 point 7)** stays exact for waits only
   tasks can end: a wait for a timer, a socket, or the stopping channel
   does not count as blocked.
6. **Signals and shutdown (core 7.5)**: SIGINT and SIGTERM are handled
   on the poller thread: the first requests a stop (`stop_requested`,
   the `stopping` channel closes, the grace timer starts, `set_grace`
   moves it), the grace or a second signal cancels main's root context,
   a third flushes output and exits with status 130. SIGPIPE is ignored.
7. **Output is flushed before a context waits**, so a program's lines
   show before it waits for a connection or a signal (stdout stays
   buffered otherwise).
8. Duration and Time lower to Int nanoseconds and Bytes to an immutable
   byte string like Str (the Go backend's representations); Clock, Net,
   Proc, and Env are capabilities; Listener and Conn are handles.
   Interpolating a Duration, Time, or Bytes is a build error for now.

## Consequences
- 2000 sleepers or 20000 tasks blocked on a channel run on one worker
  per CPU; the C test checks the pool stays that size.
- The goldens print the same on the Go, LLVM, and arm64 backends with no
  leaks, under ThreadSanitizer (its fiber API is called at every switch)
  with one, two, and one per CPU workers.
- Every native program starts the poller thread (signals), so a program
  has two threads from the start; its Xo code still runs single threaded
  until the first spawn.
- A task's stack is reserved, not committed: 10000 parked connections
  reserve about 625 GiB of address space with the default size.
- A system call that blocks (DNS in `dial`, file I/O) still holds its
  worker.
- The Linux paths (epoll, x86_64 switch) compile in principle but were
  not built or run: no Linux host was available (2026-10-08); the x86_64
  switch assembles for Darwin.
- `dial` has no timeout yet (the Go backend's is 30 s).

## Alternatives
- **Continuations**: no stacks to reserve and no switch code, but every
  function that may wait becomes a state machine in the SSA and both
  backends, with its live values in a heap frame (and their reference
  counts), and every call through a function value must assume it may
  wait.
- **Fibers only for I/O waits**: a task cannot move to a fiber once it
  runs, so it would need to start on one anyway.
- **ucontext (makecontext/swapcontext)**: deprecated on Darwin and makes
  a signal mask system call per switch.
- **A poller in each worker** (Go's netpoll in the scheduler): fewer
  thread hops for a ready socket, more code; one poller thread is
  simpler to get right first.
