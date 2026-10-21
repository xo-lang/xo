# 0318: Asynchronous preemption: a signal instead of a test in every loop

Status: Accepted (owner, 2026-10-21, with the lead's review: the signal
mask reset before the in-handler yield, the two-fibers-on-one-worker test,
no recursive frames). Drafted by Xo Lang 2. Stage 1 (arm64 darwin and
linux) in progress. Until it lands, loop polls stay on by default and
`--no-preempt` (e53f1be69) removes them for tight loop benchmarks.

## Context

Decision 0286 step 3 preempts a task that has run a slice (10 ms) while
work waits: every loop iteration of the program's code tests a word
(W_PREEMPT) and calls a stub when it is set. The test costs on loops,
measured 2026-10-21 against the same compiler with no polls (native
arm64: exact instructions under callgrind in Docker linux/arm64; LLVM:
darwin user time):

| benchmark | no polls | polls | cost |
|---|---|---|---|
| sieve, arm64 | 1,682 M | 2,111 M | +25.5% |
| sort, arm64 | 1,357 M | 1,516 M | +11.7% |
| nbody, arm64 | 2,905 M | 3,165 M | +9.0% |
| sieve, LLVM | 0.10 s | 0.18 s | +80% |
| collatz, LLVM | 0.47 s | 0.58 s | +23% |

Making the test cheaper did not help (9ccc0f0e5: the flag's address
hoisted, no memory clobber, LLVM's test in IR): what remains is the test
itself, a load and a branch per iteration in inner loops of six to eight
instructions, and on LLVM a loop with a call path is neither vectorized
nor unrolled. Against Rust, sieve went from 1.00 to 1.26x and collatz
from 1.19 to 1.42x (bench/results/compare-2026-10-21.md). The target is
the loop rows within 2% of no polls, with preemption kept.

## Decision

Preempt asynchronously, as Go has since 1.14: no test in loops; the
monitor interrupts a worker past its slice, and the interrupted thread
redirects itself into a stub that saves every register and yields.

1. **The request.** mon_scan (pool.xo) already decides which worker to
   ask (its fiber has run SLICE_NS while work waits, no worker is idle,
   and none can start). Instead of setting W_PREEMPT_I and the poll
   flag, it sends the worker's thread a signal: SIGURG on linux and
   darwin (Go's choice: no program or library is likely to use it, and
   its default action is to ignore it), with pthread_kill; at most one
   outstanding per worker (W_PREEMPT_I keeps the ask time, as now).
2. **The handler** declines, returning at once with nothing changed
   (the monitor asks again at its next scan), unless all of these hold:
   - the thread is a worker running a fiber (TLS_W, TLS_FIB);
   - the worker holds no runtime lock (W_LOCKS == 0, as preempt checks
     now);
   - the interrupted PC is in the program's code: not in the runtime
     (the runtime's functions, the modules runtime_mod names, and the
     stubs are placed in a section of their own, `__TEXT,__xo_rt` and
     `.text.xo_rt`, whose bounds the linker gives), not in the C library;
   - the fiber's stack has room above its guard for a signal frame and
     the runtime's switch (16 KiB).
   SIGURG's handler runs on the interrupted fiber's own stack (no
   SA_ONSTACK: the alternate stack stays the overflow handler's), so the
   kernel's signal frame, which holds every register the interrupted
   code had (general, flags, vector, PC, SP), is on the fiber's stack.
3. **The continuation is the signal frame.** Accepting, the handler sets
   the thread's signal mask back to the worker's normal one (a system
   call), marks the fiber async yielded, and yields from inside the
   handler (sched.fiber_yield: the fiber's own stack, holding the signal
   frame, is put aside). When the fiber runs again the handler returns,
   and the kernel's sigreturn restores every register exactly and the
   mask the interrupted code had. Nothing is pushed into the interrupted
   frame, so no register has to be free at every instruction. (Go pushes
   a call into the context and returns through R27, a register its
   compiler reserves; Xo's emitters keep values in x16 and x17 across
   instructions, the spill slot cache, so no register is free on arm64.)
   This is safe to do in a signal handler for the same reason Go's
   preemption is: the interrupted code is the program's own, outside the
   runtime and the C library, holding no runtime lock and no C library
   state, so the runtime code that runs while the fiber is set aside
   reenters nothing the interrupted code was in.
   **The signal mask.** While a handler runs, the kernel blocks SIGURG
   (and sa_mask's signals, which the runtime leaves empty) on that thread.
   Before yielding, the handler sets the thread's mask back to the
   worker's normal one (pthread_sigmask), so the worker's other fibers
   run preemptible and SIGPROF is not held; when the parked fiber runs
   again and its handler returns, sigreturn restores the mask saved in
   its frame (uc_sigmask: the interrupted code's), so nothing is left
   blocked or unblocked by mistake.
   **No nesting.** A fiber holds at most one signal frame: it is
   preempted only at a PC in the program's code; once set aside it runs
   again only by returning into its handler, whose remaining code (the
   return and sigreturn) is in the runtime section, where a SIGURG is
   declined. So a fiber preempted, resumed, and preempted again has
   always left its first handler before the second begins.
4. **Thread derived state across the interruption** (the main hazard):
   the interrupt can land between loading something that belongs to the
   worker's thread (the thread pointer, the thread local table's
   address that tl_base reuses within a block, a value read through it)
   and using it; if the task then resumes on another worker, it uses the
   old worker's state. Two ways:
   - (a) **An asynchronously preempted task resumes on the same
     worker**: the yield puts the fiber on its own worker's ring marked
     pinned (no steal, no handoff to the inject queue) until it runs
     again. Every thread derived value in registers stays valid. Cost:
     the task cannot move to an idle worker until it runs once more on
     its own; it only yields so others on that worker run, which is the
     point of preemption. Simple, nothing per instruction.
   - (b) **Go's unsafe point table**: the emitters mark the instruction
     ranges where such a value is live (tl_base's reuse, inline
     sequences that read a thread's state), and the handler declines
     there. Needs every such sequence found and kept marked as the
     emitters change; a miss is a heap corruption seen rarely.
   Choice: (a), which the signal frame continuation needs anyway: the
   frame restores registers, not the thread (the thread pointer is the
   new thread's), so a value derived from the old thread would be wrong
   after a move. Inline reference counting on shared objects (atomic
   instructions, LL/SC loops that just retry), compare and swap
   sequences, and the in-place update test (a count of 1) depend on no
   thread, so they need nothing; spin locks are only the runtime's
   (W_LOCKS); a fault's unwinding cannot be in progress (it runs in the
   runtime section).
5. **Polls go** from loops on targets with asynchronous preemption, with
   two exceptions: --race builds (ThreadSanitizer intercepts signals and
   delivers them only at its own safe points, so a spinning loop would
   never see one: race builds keep the loop polls, slower as any race
   build is), and wasm (no threads). XO_PREEMPT=0 still turns preemption
   off (the monitor sends nothing).
6. **Windows** (amd64): no signals. The monitor suspends the worker's
   thread (SuspendThread), reads its CONTEXT (GetThreadContext), makes
   the same checks as the handler (a thread suspended in the kernel has
   its RIP in ntdll: declined), copies the CONTEXT into the fiber's save
   area, and sets RIP and RSP to a park routine on the fiber's stack
   (SetThreadContext; Windows x64 has no red zone), then resumes the
   thread (ResumeThread): the routine yields, and when the fiber runs
   again it restores the saved CONTEXT with RtlRestoreContext, which the
   runtime already uses to resume after an exception, every register
   and RIP at once. Go's preemptM on Windows suspends and resumes the
   same way.
7. **Interactions.**
   - The CPU profiler (SIGPROF, decision 0238) and SIGURG are separate
     handlers. A SIGPROF while a fiber is set aside samples the thread's
     current work, not the set aside fiber; unwinding a fiber's stack
     through a signal frame (xo dump's tasks, the debugger) uses the
     kernel's signal trampoline's CFI (__restore_rt on Linux,
     _sigtramp on darwin), which the unwinders follow (stage 1 tests
     it).
   - Xo's debugger (decision 0253): SIGURG is passed to the program
     without stopping (as gdb's `handle SIGURG nostop noprint pass`, which
     Go documents too); its stepping is not interrupted, since the
     monitor does not ask while a debugger holds the thread stopped.
   - Faults: a fault while yielding cannot happen (the yield runs only
     runtime code); a fault in the program after resuming unwinds as
     before, the stub's frame being gone.
   - System calls: SA_RESTART restarts most; Xo's runtime already retries
     EINTR where it calls the kernel directly.
8. **Cost per slice.** One pthread_kill (a system call) from the monitor
   and one signal delivery (a few microseconds) per preemption, only
   when a worker has run 10 ms while work waits: about the frequency of
   today's yields (63 in a second under a loaded test), so well under
   0.1% of a worker's time.

## Stages

1. darwin/arm64 and linux/arm64: the runtime section and its bounds,
   SIGURG's handler with the checks and the yield inside it, the pinned
   requeue, mon_scan signalling, polls off for these targets (not
   --race). Tests: TestPreempt and TestPreemptMigrate unchanged (the
   spinner is preempted within the bound), a new test that interrupts a
   loop holding live values in every register (x16, x17, x30, vectors,
   flags) a thousand times and checks its result, a test of two spinning
   fibers on one worker (XO_THREADS=1) both preempted many times (each
   preemption while the other is parked inside its handler: the mask
   reset), with SIGPROF sampling both (XO_PROF=cpu), the profiler, fault,
   and dump tests, and the loop rows: sieve, collatz, sort, nbody within
   2% of no polls (native instructions under callgrind; LLVM user time).
2. linux and darwin amd64 (the same handler; the kernel places the
   signal frame below the red zone).
3. windows/amd64 by SuspendThread and RtlRestoreContext.

## Consequences

- Loops cost nothing for preemption; the poll code stays for --race.
- A runtime section on every target, which the linkers place and
  bound; the runtime's functions are found by section, not by name.
- An asynchronously preempted task is pinned to its worker until it runs
  again.
- Signals: a program that itself uses SIGURG would conflict (Go has the
  same constraint and documents it); std does not.

## Alternatives

- Cheaper polls (measured, 9ccc0f0e5): the test itself is the cost.
- Polls only in loops without inner loops: bounded latency only when
  inner loops are bounded; an unbounded inner `while` could never be
  preempted. Not the end state (interim option (B), not taken unless the
  owner asks).
- Polls every Nth iteration: a counter test costs the same two
  instructions.
- Go's unsafe point table (item 4 (b)): considered; pinning is simpler,
  has no failure mode from a missed sequence, and the signal frame
  continuation needs it anyway.
- Go's pushed call into the interrupted context: needs a register free
  at every instruction for the return (Go reserves R27); Xo's emitters
  have none (x16 and x17 carry the spill slot cache), and reserving one
  costs every function a register.
- Cooperative only (decision 0286 before step 3): a task that never
  calls the runtime starves its worker.
