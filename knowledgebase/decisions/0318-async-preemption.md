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

## Addendum (2026-10-21, before stage 1): runtime code inlined into the program

Pinning (item 4 (a)) keeps the values the interrupted code holds valid,
but while the fiber is set aside its worker runs other fibers, and they
may enter the same per thread structures: the heap's free lists (alloc
and free's fast paths), the deferred release queue, the worker's rings.
An interruption in the middle of such an update, with another fiber on
the same worker updating the same structure, corrupts it. Inside the
runtime's own functions the runtime section check declines (item 2). But
the inliner (compiler/arm64 inline.xo) inlines runtime functions into
the program's code: no rule keeps an llrt body out of a std/native or
program function, so a runtime sequence can sit in the program's text
with no section to tell it apart. Go has the same problem and marks such
code unsafe for asynchronous preemption.

So stage 1 adds the unsafe range table after all (item 4 (b), for this
case only): every instruction the emitter lowers from a value whose
inline site's callee is a runtime function (runtime_mod: the inline
sites the debug information already records) is inside an unsafe range;
the emitter writes each function's ranges, sorted, into a section of
their own (`__DATA,__xo_unsafe` and `.data.xo_unsafe`, start and end
addresses), and the handler declines when the interrupted PC falls in one
(a binary search). With pinning this covers both hazards: values derived
from the thread (pinning) and reentrancy into the worker's structures
(the runtime section and the unsafe ranges). The every-register test
gains an allocating loop (a list grown and dropped each iteration in
program code) preempted thousands of times with another allocating
fiber on the same worker (XO_THREADS=1), checked by the leak checker and
the heap's own checks (XO_HEAP_GUARD).

Sequences the emitter lowers inline, not from an inlined runtime body
(so not in the table), and why each is safe to interrupt anywhere:

- Reference count updates (dup and drop's inline paths): a count above 0
  is local, its object reachable from one task only (crossing to another
  task marks it shared, decision 0058), so a plain load, add, and store
  interrupted midway is resumed by the same task with nobody else having
  touched it. A shared count is updated atomically: one LSE instruction
  on darwin (`ldaddal`, `stadd`), and on Linux an exclusive load and
  store loop, which an interruption only makes retry (the exclusive
  monitor is cleared on the return from the signal). A drop that reaches
  zero then calls the runtime to free (the runtime section); interrupted
  between the store and the call, the object is still the task's own.
  The in place update test (a count of 1) reads a local count likewise.
- Allocation: the emitter does not lower an allocator fast path inline;
  generated code calls the runtime (xo_x_obj_*: the runtime section).
  The free list pops (alloc_fast, alloc_pop) reach the program's text
  only through the inliner, which the table covers.
- Stack checks: native functions have no stack limit check in their
  prologues (fiber stacks have guard pages); a check added later reads
  the fiber's own limit, which pinning keeps valid.
- Thread local table reads in std's native modules (rt.tlocal_get
  outside the runtime): pinned to the same thread, the table is the
  same.

## Addendum (2026-10-21): stage 1 as built (arm64 darwin and Linux, native backend)

- **Tables, not a runtime section.** The emitter (compiler/arm64
  apreempt.xo) brackets unsafe code with local `_du<2k>` and `_du<2k+1>`
  labels, which every line pass keeps as rows (lsr.xo is_row_label). A
  runtime function (Func.file under native/llrt, native/rt.xo, or
  native/sys_*: runtime_mod's modules) is one range, prologue and fault
  stubs included. In a program function a block's range runs from its
  first instruction whose inline site chain (InlSite.file, .parent)
  reaches a runtime file to its last, everything between included, with
  one instruction more on each side (pair_mem moves the rows between the
  two halves of a pair before it). Each file of a build (each part of a
  split build, cache hits included, since the labels are in the placed
  text) gets `xo_unsafe_<ns>`: its functions' text start, its size, the
  range count, and the ranges as 32 bit offset pairs in order; the last
  file lists them in `xo_unsafe_parts`, held in static word 77. The last
  file's code (entry, exports, count helpers, the scheduler's stubs) is
  in no table, so it is never interrupted, and neither are the C library
  and the signal trampolines. An unpaired label fails the build. Ranges
  are found by a binary search per file.
- **Switch.** On for `--backend=arm64` (the native default on arm64)
  targeting darwin or linux, not `--race`; `XO_ASYNC_PREEMPT=0` keeps the
  loop polls (decision 0286 step 3), `--no-preempt` and
  `XO_PREEMPT_POLLS=off` turn both off. Keyed in the front end, function
  and part caches (`async=`). LLVM keeps its polls (its inliner keeps no
  provenance for the tables).
- **Runtime (pool.xo).** The monitor installs SIGURG's handler
  (SA_SIGINFO, SA_RESTART, on the interrupted stack) when it starts; each
  worker records its thread (pthread_self, gettid) in its loop's start
  (worker_main, root_main). An ask (mon_scan) sends pthread_kill or
  tgkill, and is sent again each tick while it is out. The handler
  (cabi.xo on_urg, pool.async_signal) declines unless the thread runs a
  fiber in a worker loop with no runtime lock, an ask is out, the PC is in
  a table's text outside its ranges, and the stack has 48 KiB above the
  mapping's start; then it serves the ask, unblocks SIGURG (the worker's
  other fibers stay preemptible; sigreturn restores the interrupted
  mask), and yields. run_fiber puts such a fiber on its worker's pinned
  list (W_PIN, linked by F_NEXT), which take_local reads after the ring.
  A pinned fiber counts as waiting work for its worker in mon_scan: when
  it did not, a fiber preempted while another took its place waited for
  that one to end (2 preemptions instead of about 100 in the register
  test).
- **Measured.** Instructions on linux/arm64 (callgrind, Docker):
  sieve 1,682,418,179 against 1,682,418,157 with `--no-preempt`
  (2,110,917,208 with the polls), sort 1,356,668,664 against
  1,356,668,642 (1,515,515,249), nbody 2,905,036,037 against
  2,905,035,449 (3,165,035,146), fib equal; collatz (no callgrind count)
  0.51 to 0.53 s user against 0.50 to 0.53 on the Mac. Tests
  (internal/llvm TestAsyncPreempt, darwin and Linux): every register
  restored across 1,800 to 2,900 interruptions per 20 runs; an
  allocating loop interrupted 640 to 1,150 times per 100 runs with the
  leak check and XO_HEAP_GUARD; the profiler's SIGPROF alongside;
  TestPreempt, TestPreemptMigrate (plain and --race) unchanged.
- **Limit.** An inlined runtime value with no line takes its call's
  site (inline.xo), so it counts as program code; inside a sequence the
  first-to-last span covers it, so only a runtime sequence whose first
  or last side effect has no line would escape (stores and atomics come
  from source lines). A loop whose every block is inlined runtime code
  is not preemptible (a runtime loop, bounded).

## Addendum (2026-10-21): stage 2 (amd64, linux and darwin)

The x86 emitter (x86emit.xo) brackets the same spans; its text passes
treat `_du` labels as boundaries (they are not x86_dbg_label's debug
rows), so no fused run crosses one. The handler reads x86_64's rip and
rsp (Linux gregs 15 and 16 at 40 in the ucontext, darwin's
x86_thread_state64 words 7 and 16), and Linux uses x86_64's gettid,
getpid, and tgkill. On for `--backend=amd64` targeting linux or darwin
(windows/amd64 is stage 3). linux/amd64 in Docker (emulated): preempt
10/10, every register restored across 7,240 interruptions in 20 runs,
the allocating loop 0/100 failures (751 interruptions, XO_LEAKCHECK and
XO_HEAP_GUARD); instructions sieve 1,845,609,533 against 1,845,609,512
with `--no-preempt`, sort 1,847,673,823 against 1,847,673,802.
Goldens async_preempt_regs and async_preempt_alloc (XO_THREADS=1) run
the stress programs on every backend and are in the Docker amd64
sample. darwin/amd64 is untested (no Intel Mac in the gates: best
effort).

## Addendum (2026-10-21): the LLVM backend (stage 4; Accepted, owner 2026-10-21)

Owner and lead, 2026-10-21: LLVM builds go poll-free too. Xo lang 4's
measurement: nbody at O3 with the approved list fixes runs 1,169 M
instructions without polls and 2,130 M with them; the poll's inline asm
call keeps LLVM from promoting the loop's values to registers, and an
`unordered` flag load recovers only 16% of it.

The native backend's tables do not carry over: LLVM inlines runtime
code into the program's functions by design (the backend calls
xo_x_obj_rl_new_n, rl_free_n, rc_defer_n, and others with
`alwaysinline`, and its own inliner takes any small runtime function),
and after optimization nothing marks which instructions came from where.
Keeping runtime code out of program functions would give up the
allocation fast paths. Reading the DWARF inline records of every object
before the link would work but needs debug information in every build
and a DWARF reader in the driver.

Instead, Go's way (its `acquirem` and `mallocing`): the runtime says
when it must not be interrupted, with a count kept per fiber.

1. **Which runtime functions guard.** The SSA builder marks every
   runtime function (runtime_mod's modules) that can reach a thread's
   own structures: one that uses rt.tlocal_get, rt.tlocal_set, or
   rt.tlocal_addr, or calls (transitively, over the module's call graph)
   one that does. The allocator, the free lists, the deferred release
   queue, the worker's rings, and the scheduler's switches all go
   through the thread local table, so they are marked; string compares,
   hashing, and list arithmetic are not. The signal entries (cabi.xo
   on_urg, on_prof) are not marked, or the handler would always see its
   own count.
2. **The guard.** A marked function's LLVM body starts by adding one to
   the current fiber's no-preemption count (the fiber record's new word
   F_NOPRE, through the fiber pointer in the thread local table, read
   inline, not through the noinline xo_tlocal_get) and ends, on every
   return, by taking one away through the same fiber pointer, so a
   function that parks and resumes on another thread still decrements
   its own fiber. The count's updates are volatile, and an
   `asm sideeffect "", "~{memory}"` follows the increment and precedes
   the decrement: an opaque instruction that may read and write all
   memory, which LLVM moves no load or store across (stronger in practice
   than a `fence syncscope("singlethread")`, which orders atomic
   operations; xo_tlocal_get relies on the same barrier), so no load or
   store of the guarded body moves out when LLVM inlines it. A thread with no
   fiber skips both (it is never preempted). The count is per fiber,
   not per thread: a fiber that parks inside a marked function (a
   channel wait inlined into the program) leaves its own count up while
   others run on the thread.
   Not guarded, and kept out of program functions instead (`noinline`):
   a marked function that runs program code or switches stacks (a
   closure call, rt.sched_run_task, rt.stack_switch: the task runners
   and fiber_run, which would otherwise hold the count for a whole task
   body), and one that can raise (an unwind would skip the decrement).
   The handler's own entries (on_urg, on_wpark, async_signal,
   async_wpark) are exempt.
3. **The handler** (unchanged otherwise) declines when the current
   fiber's F_NOPRE is not 0, and when the PC is outside the program's text: in LLVM builds the
   program's functions (not the runtime's) are placed in a section of
   their own (`__TEXT,__xo_text` on darwin, `xo_text` on ELF, whose
   `section$start$` and `__start_`/`__stop_` symbols the linkers
   define), and static word 77's list holds one table: that section, no
   ranges. Runtime functions not inlined stay in the ordinary text, so
   they are declined as the native backend's are. The C library, compiler-rt, and the signal trampolines are
   outside it, so an interrupted malloc or memmove is declined.
4. **Polls off** for `--backend=llvm` on darwin and Linux, not --race
   (ThreadSanitizer, item 5). Windows LLVM follows stage 3.
5. **Cost.** A marked function inlined into a program loop costs the
   fiber pointer's load, an add, a store, and the same at the end (about
   six instructions per allocation or release that reaches the
   allocator), and its barrier ends the loop's register promotion only
   where the loop already calls into the allocator.
6. **Tests and measurements.** TestAsyncPreempt and the goldens
   async_preempt_regs and async_preempt_alloc on the LLVM backend (darwin
   and linux/arm64), TestPreemptMigrate, the LLVM goldens at -O1 and the
   -O2 sample, and the loop rows (sieve, collatz, sort, nbody) and an
   allocation heavy row (push, update) against `--no-preempt`, with
   Xo lang 4's nbody kit.

Estimate: about half a day of work: the builder's marking and the
guard's emission (compiler/ssa, compiler/llvm), the fiber word and the
handler's two checks (std/native/llrt pool.xo, sched.xo), the section
attribute and its table, then the tests and measurements; the full LLVM
goldens through the heavy queue.

## Addendum (2026-10-21): stage 3 (windows/amd64)

No signals: the monitor suspends the worker's thread (SuspendThread),
reads its CONTEXT (GetThreadContext, CONTEXT_FULL) into a save area at
the top of the running fiber's stack (2,560 bytes above the fiber's
first frame, committed with the stack's top, so the monitor never
touches the guard page; fiber_frame's size and commit are reduced by
it, so the TEB's StackLimit stays exact), makes the handler's checks
(the fiber the worker runs, W_RUNF; no runtime lock; the PC in a table's
text outside its ranges; stack room), and sets a copy's RIP to the park
routine (cabi.xo on_wpark), RSP 256 bytes below the interrupted one at
a call's alignment (Windows x64 has no red zone), RCX the saved CONTEXT
(SetThreadContext, control and integer only), then resumes the thread.
The routine yields; when the fiber runs again RtlRestoreContext restores
every register, the flags, and the vector state. Workers open a handle
to their own thread (OpenThread) for the monitor.

Measured on a real Windows host (CI run 39657249703, windows-batch.sh
step 5, `--backend=amd64`, XO_THREADS=1, three runs each):
async_preempt_regs ok with 49, 51, 49 preemptions, async_preempt_alloc
ok with 3, 3, 2, the preempt program ok with 9, 5, 7. Under Wine with
Box64 the first two preemptions work and then Wine's suspend machinery
stalls the monitor (a GetThreadContext fails and the monitor's
SuspendThread never returns), so Wine is not a test bed for this.

The allocating program's counts are low on every platform (about 7 per
run on darwin and Linux, 3 on Windows, against about 100 for the
register program): its loops spend most of their time in the allocator
and in runtime code inlined into them, where the handler declines and
the monitor asks again at its next tick. Expected, and the reason the
test counts preemptions over three runs.
