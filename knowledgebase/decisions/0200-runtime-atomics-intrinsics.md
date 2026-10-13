# 0200: Atomics and memory ordering intrinsics for the runtime

Status: Accepted (2026-10-13), implemented on the atomics branch:
decision 0195 prerequisite 1. Internal only (decision 0074): no
language, spec, or std surface change. Evidence:
`bench/results/atomics-2026-10-13.md`.

## Context

Decision 0195 wants no compiled C in the runtime; its audit
(`bench/results/zero-c-audit-2026-10-13.md`, "(1) Atomics and memory
ordering") found 4,966 lines of std/llrt that need only atomics (and
system calls) to move, and sketched intrinsics on `List[Int]` slots with
constant orders. Until now Xo code could not express an atomic at all:
Atomic[T] was six calls into xo_rt.c.

## Decision

1. **Intrinsics** in std/native/rt.xo, visible only to std's
   implementation and runtime modules (`use native/rt`):
   `slot_load[T](xs, i, ord) -> Int`, `slot_store(xs, i, v, ord)`,
   `slot_swap(xs, i, v, ord) -> Int`, `slot_cas(xs, i, old, new, ord) ->
   Bool` (strong), `slot_add(xs, i, d, ord) -> Int` (fetch add, wrapping),
   `slot_addr(xs, i) -> Int` (for futex style waits), `fence(ord)`,
   `spin_hint()`. ord is a constant: 0 relaxed, 1 acquire, 2 release, 3
   acq_rel, 4 seq_cst (C11's); a non constant, or an order an operation
   cannot take, rounds up to seq_cst. The cell is word i after a three
   word header: element i of a `List[Int]` made at its final size (no
   copy on write; decision 0058's shared count hands it to threads), or
   an Atomic, whose value is slot 13 (byte 128) of the C runtime's
   object. The container parameter is generic so that an Atomic handle
   can be one; unchecked like `bytes_word`.
2. **Names**: `slot_*`, not the audit's `atomic_*`: `rt.atomic_add`,
   `atomic_cas`, `atomic_swap` already name the SSA builders' calls of
   the C Atomic (gotchas/native-rt-names-shared-with-ssa-calls.md).
3. **Lowering**, inline in every emitter, Xo first and mirrored in Go
   for the differential tests (identical output, TestSelfhostArm64 and
   TestSelfhostLLVM):
   - selfhost/arm64 and internal/arm64: relaxed `ldr`/`str`, else
     `ldar`/`stlr`; Darwin LSE `swp`, `cas`, `ldadd` with the order's
     `a`/`l`/`al` suffix; Linux (ARMv8.0 baseline) calls of libgcc's
     outline atomics `__aarch64_{swp,cas,ldadd}8_<model>`, which is what
     the C runtime compiled with `-moutline-atomics` calls (with cc they
     test for LSE at run time; selfhost/macho's ELF linker writes LL/SC
     ones); `dmb ishld` for an acquire fence, `dmb ish` otherwise;
     `yield`. The MIR marks them `keep` (dce, live_dce, and thread_cset
     treated every plain instruction with a result as pure) and early;
     emit_plain gives a spilled early result its own scratch register
     (gotchas/arm64-plain-defs-assumed-pure.md). selfhost/macho
     assembles swp, cas, dmb, yield.
   - selfhost/llvm and internal/llvm: `load atomic`, `store atomic`,
     `atomicrmw xchg|add`, `cmpxchg` (failure ordering the strongest
     allowed), `fence`; the target attributes of decision 0119 pick the
     instructions (LSE on darwin/arm64, outline atomics on Linux arm64,
     locked instructions on x86-64). `spin_hint` is nothing there: the
     IR is target independent.
   - selfhost/wasm (one thread): plain loads and stores on a list cell;
     on an Atomic, the operations go back to rt3.xo's `$atomic_*`, which
     are scheduling points (gotchas/wasm-atomic-ops-are-scheduling-points.md).
   - The SSA builders need no new effect: an `rt.` call is already never
     merged, hoisted, or forwarded.
4. **Pilot port: Atomic[T]** under decision 0193's mechanism. get, set,
   swap, and compare_and_swap become one seq_cst cell intrinsic each in
   the SSA builders (selfhost/ssa conc.xo `atomic_op`, internal/ssa
   build_conc.go; also the Debug text of an Atomic); add (overflow
   faults at the caller and stores nothing) and max are
   std/native/llrt/atomic.xo, called through the native prelude's
   `llrt_atomic_add` and `llrt_atomic_max`. `xo_atomic_new` stays C (the
   allocator is prerequisite 2). The C operations moved under `#ifdef
   XO_ATOMIC_C`; `XO_LLRT_C=atomic` (Go driver) loads atomic_c.xo and
   makes the Go SSA builder call them, and IOFlags (and xo1's io_flags)
   add `-DXO_ATOMIC_C`. **The bar holds** (bench/perf/atomic, whole
   process instructions, Xo over C): 0.596x to 0.937x on arm64, 0.711x
   to 0.827x on LLVM for every workload; so the Xo path is the default
   and the C path may be deleted under decision 0198.
5. **Tests**: the golden atomic_ops (every operation, eight contended
   tasks, the overflow fault) on the Go backend, LLVM, arm64, wasm, and
   ThreadSanitizer (TestGoldenTasks), with either path; bench/litmus
   (store buffering, message passing, CAS, fetch add, swap lock, and
   Atomic.add counters) through a test only std module added to a copy
   of std/, on an Apple M4 Pro, a Graviton, and x86-64.

## Consequences

- Runtime code in Xo can now synchronize: the parks, budgets, handle
  states, and the event loop's flags of the audit's (1) are portable
  once their system calls (prerequisite (3)) are.
- Atomic[T] costs no call on the native backends (get is one `ldar`).
- A cell is a word of a fixed size list or of an object whose layout the
  emitters share with C (`_Static_assert` in xo_rt.c); raw addresses
  wait for prerequisite (2) (`rt.atomic_cas_at`).
- Linux arm64 read modify writes are calls, as in the C runtime; inline
  LL/SC or LSE behind a feature check is a later choice.

## Alternatives

- Inline LDAXR/STLXR loops on Linux arm64 (as the reference count
  helpers do): needs two or three results per MIR instruction, which
  emit_plain's spill scratch registers cannot hold with every operand
  spilled; the outline helpers match the C runtime and use LSE on
  Graviton.
- Atomics on raw addresses now: needs prerequisite (2)'s address and
  layout rules; slots of lists need neither.
- Port Atomic as an Xo struct holding a `List[Int]`: changes the handle
  kind every emitter, the reference counts, and wasm know; the cell
  view of the C object changes none of them.
- A spin hint in LLVM through target intrinsics: needs the target in the
  emitter (it has none); next step if a spin loop shows up in a profile.
