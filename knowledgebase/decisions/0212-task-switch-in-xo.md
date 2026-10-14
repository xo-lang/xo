# 0212: The task switch and scheduler core in Xo

Status: Accepted (2026-10-13), stages 1 and 2 implemented: decision
0195 prerequisite 6 (the audit's group (6)), internal only (decision
0074: no language or spec change). The task switch, the thread locals,
the fibers' life, the run queue ring, and the task states (stage 1);
the worker pool, the locks' slow paths and futex waits, worker threads,
fiber stacks, parks, the cancellation tree, a spawn's link and queueing,
and a scope close's help and join (stage 2, std/native/llrt/pool.xo)
are Xo by default; `XO_LLRT_C=sched` keeps the C path until a full tier
passes on main. Await, the wait lists, the task's record and end,
contexts, and budgets stay C (what blocks each: Stage 3).
Files: `std/native/rt.xo` (the intrinsics), `std/native/llrt/sched.xo`,
`sched_c.xo`, `cabi.xo` (the exports); the emitters (internal/arm64
sched.go and isel.go, internal/llvm sched.go and fromssa.go,
selfhost/arm64 sched.xo and calls.xo, selfhost/llvm sched.xo and
call.xo); selfhost/macho (enc.xo, asm.xo, elf.xo: TLV and local exec
TLS); selfhost/ssa contract.xo (the no allocation check); std/llrt/xo_rt.c;
internal/build tzdata.go and selfhost/xo link.xo (-DXO_SCHED_C).
Stage 2: `std/native/llrt/pool.xo`, cabi.xo, rt.xo (`rt.sched_rt`),
sys_darwin.xo and sys_windows.xo (the platform calls), std/llrt/xo_rt.c.
Evidence: `bench/results/task-switch-2026-10-13.md`.

## Context

The zero-C audit (bench/results/zero-c-audit-2026-10-13.md, "(6) Task
switch and thread state") found 1,166 lines of xo_rt.c held by the per
CPU fiber switch (assembly per CPU and OS), fiber stacks, the current
context and worker thread locals, and the heap pointer; the scheduler
around them (run queues, parks, tasks) needs (1) atomics and (2) raw
memory, both done (decisions 0200, 0207), and the C may call Xo
(decision 0204). The allocator moved to Xo (decision 0207 stage 3) with
its heap pointer left a C thread local until thread locals exist.

## Decision

1. **Intrinsics** (std/native/rt.xo, for std/native/llrt only):
   `rt.stack_switch(save, to)`: the C runtime's xo_fiber_switch (saves
   the callee saved registers on the stack, stores the stack pointer at
   `save`, continues on the stack at `to`); `rt.fiber_start_addr()`: the
   address of the start sequence (calls the frame's second register
   slot with the first); `rt.tlocal_get(k)`, `rt.tlocal_set(k, v)`, and
   `rt.tlocal_addr()`: word k of a 16 word per thread table, and the
   table's address (valid while the code stays on its thread).
2. **Lowering.** They are calls of functions the emitter puts in the
   program at the first use (`xo_stack_switch`, `xo_fiber_entry`,
   `xo_tlocal_get`, `xo_tlocal_set`, `xo_tlocal_addr`, the table
   `xo_tlocal`), never inlined: a fiber may go on on another thread after
   a switch, and a compiler keeps a thread local's address within a
   function. The SSA builders need nothing new (an `rt.` call is a call
   of `xo_<name>`); `rt.fiber_start_addr` is an address constant.
   - arm64 emitters (darwin, linux): the sequences as assembly
     (AAPCS64: x19 to x28, fp, lr, d8 to d15; the same as xo_rt.c's).
     The table is a Mach-O TLV (`__thread_vars` descriptor,
     `__thread_data` template; access through the descriptor's thunk,
     which keeps every register but x0, x16, x17, and lr, so the
     functions need no frame) and an ELF local exec TLS symbol
     (`mrs tpidr_el0` plus `:tprel_hi12:` and `:tprel_lo12_nc:`).
     selfhost/macho assembles them byte identical to clang (TLVP
     relocations, the thread local section types, `mrs`, TLS
     relocations, STT_TLS).
   - LLVM emitters: naked functions whose bodies are the C runtime's
     sequences per target (arm64 AAPCS64; Windows on Arm with the TEB's
     StackBase, StackLimit, DeallocationStack; x86-64 System V with
     MXCSR and the x87 control word; Windows x64 also rdi, rsi, xmm6 to
     xmm15, and the TEB), and a `hidden thread_local` global that one
     internal function names (a split build, decision 0168, copies a
     global into each module that uses it). Each thread local function
     starts with an empty asm that clobbers memory, or LLVM infers
     `xo_tlocal_addr` readnone and merges its calls across a switch
     (gotchas/llvm-thread-local-address-cse.md). Not module asm: the split
     copies the module header to every module.
   - wasm: none (no exports, no fibers; its runtime runs tasks to
     completion).
3. **Thread state.** The C's `cur`, `tls_fib`, and `tls_w` are words 0,
   1, and 2 of the table (the C declares `extern __thread int64_t
   xo_tlocal[16]` and reads it in its noinline accessors, at the cost its
   own `__thread` had); word 3 is the heap pointer (decision 0207 stage
   3's `tls_heap`, which waited for this), 4 to 15 unused.
4. **The scheduler core in Xo** (std/native/llrt/sched.xo, C ABI exports
   in cabi.xo): the run queue ring (put, get, a thief's half, a batch;
   head and tail are 64 bit counts now, in both paths), the first frame
   of a fiber per target, fiber_main (the start: the task, then the
   switch back for good), fiber_park, fiber_parking, fiber_wake, and
   fiber_run (the parking handshake on the fiber's state, the switches,
   the thread locals), task_claim, task_dequeued, and scope_units_done.
   The layouts are xo_rt.c's, asserted there with `_Static_assert`
   against sched.xo's constants. The no allocation, no counting rule of
   decision 0207 (selfhost/ssa contract.xo check_no_alloc) holds for the
   module: Int words, wrapping arithmetic, calls only of intrinsics and of
   three C hooks (`rt.sched_hook`, `rt.sched_run_task`, the C path's
   `rt.sched_c`); `target`'s fields are admitted as scalar tags.
5. **Hooks.** The sanitizers' fiber calls (ThreadSanitizer's
   `__tsan_switch_to_fiber`, AddressSanitizer's start and finish switch)
   and the scheduler statistics stay C, in `xo_sched_hook`, which the Xo
   code calls around a switch only when the fiber's hooks word is set
   (sanitizer builds, XO_RTSTATS): one load and branch otherwise. The hook
   is not instrumented (gotchas/tsan-fiber-switch-in-callee.md).
6. **What stays inline in C**: the thieves' ring peek (rq_nonempty), in
   every steal round of a spinning worker, where a call cost the arm64
   ping pong 9.7 percent more instructions per round trip (4.7 after it
   moved back; gotchas/scheduler-instruction-counts-spin.md).
7. **C path**: `XO_LLRT_C=sched` loads sched_c.xo, whose functions call
   `rt.sched_c(op, ...)`; both drivers add -DXO_SCHED_C when generated
   code names `xo_sched_c`, and xo_rt.c then compiles its own switch
   (`xo_fiber_switch`, `xo_fiber_start`), thread locals, ring, and fiber
   code (427 lines compiled only there). The emitted start is named
   `xo_fiber_entry` so the C only tests, which link the default exports
   with either C, never see two definitions.

## Stage 2 (2026-10-13): the pool, locks, parks, stacks, threads, spawn and join

Implemented in `std/native/llrt/pool.xo` (C ABI exports in cabi.xo),
over xo_rt.c's records (`pool`, `xo_worker`, `fibers`, `xo_fiber`,
`xo_park`, `xo_node`, `xo_ctx`, `xo_scope`, `xo_task`), whose layouts
are asserted against pool.xo's constants with `_Static_assert`. The C
path is the same switch: `XO_LLRT_C=sched` (-DXO_SCHED_C) compiles
xo_rt.c's own copies and calls none of pool.xo. No compiler change: the
new C services are one `rt.sched_rt(op, a, b)` declaration (a call of
`xo_sched_rt`, lowered like every `rt.` call), and the platform calls
are new declarations in std/native/sys_darwin.xo and sys_windows.xo
(decision 0213's mechanism).

1. **Locks and futexes.** `xo_lock` is a 64 bit word (the slot atomics
   are of words; a 32 bit word beside other fields could not be taken
   from Xo), 0 free, 1 held, 2 held with sleepers, the futex word its
   low half. The C keeps the first try and the release inline (a call
   per lock in every channel operation); the slow path (`lock_slow`:
   100 spins, then the futex) and the wake are Xo: `futex_wait` and
   `futex_wake` call `__ulock_wait`/`__ulock_wake` (libSystem),
   `WaitOnAddress`/`WakeByAddressSingle` (the compare value in a scratch
   word of the thread local table, word 4), and futex(2) through
   linux.xo's `rt.sys6`. Inside pool.xo the first try is written at
   each site (the arm64 emitters did not inline a `lock` function whose
   slow path is a call).
2. **The pool**: worker records (made with `rt.heap_large`, a word
   `pool` and a scratch batch of 129 words added to each, for a steal's
   half, the inject queue's share, and a full ring's half: the C kept
   them on its stack), wakep, the inject queue (its buffer from
   `rt.heap_large`), rq_put's move of a full ring's half, pool_push,
   take_local, steal_round (the thieves' peek inline, as stage 1's C),
   any_work, find_work, go_idle, and the worker's run loop
   (`worker_main`, the thread's start routine). The C keeps the pool's
   setup (pool_init: XO_THREADS, XO_SPIN_ROUNDS, the stacks' size), the
   root threads' records (root_worker), and the deadlock check
   (dl_check_locked, under the pool's lock, which Xo takes), called
   through `rt.sched_rt` with the statistics, the monotonic clock, and
   the test world's clock kick.
3. **Worker threads**: `pthread_create` with a 64 MiB stack, detached,
   from Xo on macOS (sys_darwin: pthread_attr_init, setstacksize,
   setdetachstate, create, destroy; the attributes in an
   `rt.heap_large` block), `CreateThread` with
   STACK_SIZE_PARAM_IS_A_RESERVATION and `SetThreadStackGuarantee` in
   the new thread on Windows (what xo_win.c's `_beginthreadex`
   trampoline did), and on Linux pthread_create through `rt.sched_rt`:
   the C library's in a libc build (a raw clone would skip its thread
   setup, which the C runtime's libc calls need), and in an XO_NOLIBC
   build decision 0206 stage 2's, which is Xo (cabi_nolibc.xo
   thread_create over `rt.sys_clone`).
4. **Fiber stacks**: fiber_new and fiber_free (the worker's cache of 8,
   the shared cache of 64 under its lock, the mapping): `mmap` and
   `mprotect` of a guard page (libSystem; linux.xo's system calls),
   `VirtualAlloc` reserve plus the committed top and a PAGE_GUARD page
   (Windows), `munmap`/`VirtualFree`; the fiber record from
   `rt.heap_large`. With the hooks word set (sanitizer builds,
   XO_RTSTATS) Linux stacks come from the C library's mmap: a raw
   system call bypasses ThreadSanitizer's interceptor
   (gotchas/tsan-blind-to-xo-synchronization.md). The counts
   (fiber_live, fiber_peak), ThreadSanitizer's fiber records, and the
   failure message stay C services.
5. **Parks and the cancellation tree**: park_reset, park_signal (a
   fiber's wake and queueing, a sleeping root's futex wake, the
   deadlock counts), park_wait (a fiber parks; a root thread helps with
   queued work, then sleeps; counted and uncounted waits), node_link,
   node_unlink, node_cancel (the parent first walk), node_set_waiter.
   The C wrapper of a wait flushes output and pauses the budget around
   it (decision 0175), passes the test world's rule (every wait
   counts), and finds the context's fiber or its root thread's record
   (each a C service call per wait when Xo asked for them).
6. **Spawn and join**: a spawn's link into its scope, its pending
   count, its node, the list and its pruning, and its queueing
   (`scope_add`); the close's help with the tasks nobody started and
   its wait for every task and queue entry (`scope_join`). The C keeps
   the records (the task's allocation and header, its closure and
   result, budgets, fault text), the fault of a spawn on an ended scope,
   the cancellation the close mode asks for, and the release of the
   list.
7. **ThreadSanitizer**: the Xo code is not instrumented, so the C tells
   it about the hand offs through Xo: `__tsan_release` before a park
   signal and a spawn, `__tsan_acquire` after a wait, a join, and
   before a task runs; after a lock's slow path an acquire load of its
   word.

Stage 2's bar and correctness: `bench/results/task-switch-2026-10-13.md`
("Stage 2").

## Stage 3 (open): what stays C

- **Await and the wait lists** (xo_task_await, wl_wait_as, the
  channels', selects', and mutexes' waits): a wait list entry lives on
  the waiter's C stack; Xo has no stack slots for a record, and an
  allocation per wait is what the C avoided. Next: an entry per context
  (xo_ctx holds at most one wait at a time), then await moves.
- **The task's record and its end** (xo_task_spawn's allocation and
  header, xo_task_done, xo_task_unwound, run_claimed): the entry is
  called through a function pointer (decision 0206 stage 2's
  `rt.call_c2` can now), results and closures are
  released through drop functions, and fault text is malloc'd C.
- **Contexts and budgets** (ctx, the root context, bud_*), the deadlock
  check and the test world's clock (dl_check_locked, tw_kick): the
  records the C runtime's other parts read.
- **Linux thread start in libc builds** (pthread_create through the C
  library).
- **The pool's setup** (getenv, sysconf) and the statistics' table.

## Consequences

- No assembly in xo_rt.c on the default path: the compiler emits the
  switch for every target it builds.
- Every native program carries the scheduler exports (cabi.xo) and the
  switch and thread local functions; the SSA goldens and the arm64
  assembly goldens grew again, and the recorded Go outputs of the
  selfhost differentials (internal/selfhost/testdata/gogoldens) need
  recording again (scripts/selfhost-record-goldens.sh).
- The C reads the thread locals of the program's code: a C only test
  must link a program's IR (internal/llvm cOnly), as for the other
  exports.
- `target` matches are not folded by the SSA passes, so fiber_frame's
  four frames are all compiled; the arm64 emitter leaves a few compares
  of constants (once per fiber start).

## Alternatives

- **Module level asm in the LLVM emitters** (the audit's sketch): the
  split build copies the header into every module, so the switch would
  be defined once per module; naked functions are ordinary definitions.
- **pthread keys for thread locals** (Go's darwin approach): no
  assembler or linker work, but an initialization at every thread start
  and an extra indirection; TLV and local exec are what the C runtime
  already uses, and the linkers already handle them.
- **A reserved register for the current context** (the audit's later
  variant, Go's g): an internal ABI change in both arm64 emitters;
  worth it only once no C reads the thread locals.
- **Inline thread local access in the emitters**: an LLVM function keeps
  a thread local's address across the switch's opaque call (it treats it
  as constant within the function), so the access stays a call; on the
  scheduler's own stack `rt.tlocal_addr` once per call is enough.
- **Moving only the switch and leaving the fiber code C**: the switch
  alone is a few lines; the parking handshake and the fiber's life are
  where the scheduler's correctness lives, and they need only the
  intrinsics above.
