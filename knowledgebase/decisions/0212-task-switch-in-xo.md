# 0212: The task switch and scheduler core in Xo

Status: Accepted (2026-10-13), stage 1 implemented: decision 0195
prerequisite 6 (the audit's group (6)), internal only (decision 0074: no
language or spec change). The task switch, the thread locals, the
fibers' life, the run queue ring, and the task states are Xo by default;
`XO_LLRT_C=sched` keeps the C path until a full tier passes on main.
Spawn, done, await, scope close, the inject queue, idle workers, and
fiber stacks stay C (what blocks each: Stage 2).
Files: `std/native/rt.xo` (the intrinsics), `std/native/llrt/sched.xo`,
`sched_c.xo`, `cabi.xo` (the exports); the emitters (internal/arm64
sched.go and isel.go, internal/llvm sched.go and fromssa.go,
selfhost/arm64 sched.xo and calls.xo, selfhost/llvm sched.xo and
call.xo); selfhost/macho (enc.xo, asm.xo, elf.xo: TLV and local exec
TLS); selfhost/ssa contract.xo (the no allocation check); std/llrt/xo_rt.c;
internal/build tzdata.go and selfhost/xo link.xo (-DXO_SCHED_C).
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
     global into each module that uses it). Not module asm: the split
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

## Stage 2 (open): what the rest needs

- **Task spawn, done, await, scope close** (xo_task_spawn,
  xo_task_done, xo_task_unwound, xo_task_await, xo_scope_close): they
  take the runtime's locks (`lk`/`unlk` over `__ulock_wait`, futex,
  WaitOnAddress: decision 0206 stage 2 and decision 0213's platform
  calls), allocate (the heap is Xo since decision 0207 stage 3, through
  C), call the task's entry through a function pointer (an `rt.call_addr`
  intrinsic, as decision 0207 found for the free walkers), and keep
  fault text in malloc'd C strings. Next: `rt.call_addr`, then the locks
  over `rt.slot_*` plus the platform wait and wake.
- **The inject queue, idle workers, wakep, steal_round**: the pool's
  global record and its lock; with the locks above, they move with
  `rt.static_word` (decision 0207's design) for the pool's words.
- **Fiber stacks** (mmap, mprotect, VirtualAlloc, the caches): platform
  calls (decision 0213); Linux could use `rt.sys6` now.
- **Worker threads**: thread creation is decision 0206 stage 2 (Linux)
  and 0213 (pthread_create, _beginthreadex). The heap pointer is word 3
  of the table already; alloc.xo can read it with `rt.tlocal_get` once
  heap creation (heap_slow: calloc and the heaps' mutex) moves too.

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
