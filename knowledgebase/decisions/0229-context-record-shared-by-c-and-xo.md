# 0229: The context record shared by C and Xo

Status: Accepted, implemented (2026-10-14, B17 of the zero-C audit,
decision 0212 stage 3; std/native/llrt/ctx.xo and budget.xo;
measurements in bench/results/contexts-2026-10-14.md).

## Context

A running context (the program's main or a task) is xo_rt.c's `xo_ctx`.
Its fields are the cancellation node, the unwinding flag, the shield
depth, the parking record, the Mutexes held, the network outcome, the
fault record, the unwinding boundary and exception object, and the
budget's fields. The scheduler items that come after B17 (B18 wait
lists, B19 channels and Mutexes, B20 the pool) each move code that reads
or writes these fields.
- Before this decision, fault.xo (B06), pool.xo, and sched.xo each copied
  the few offsets they needed, each with its own assert.
- Generated code reached the context only through C entry points:
  xo_shield_begin, xo_shield_end, xo_shield_abort, xo_unwinding,
  xo_tick, xo_budget_open, xo_budget_end.

## Decision

1. **One table of the layout**: std/native/llrt/ctx.xo `C_*` names
   every field of xo_ctx (224 bytes): node 0, unwinding 8, shield 12,
   park 16, held 48, nheld 56, capheld 60, net_status 64, net_msg 72,
   faulting 80, fault_msg 88, fault_loc 96, fault_detail 104, boundary
   112, fault_exc 128, bud 160, bud_left 168, bud_q 176, bud_run 184,
   bud_amark 192, bud_heap 200, bud_paused 208.
   - xo_rt.c asserts every one, and the size (`ctx.xo C_*`, `ctx.xo
     C_BUD_*`).
   - A new module takes its offsets from ctx.xo.
   - A field added to xo_ctx gets its C_* constant and its assert in the
     same change.
   - Fields are word aligned. A field Xo writes with the word atomics
     (rt.slot_*) is 8 bytes, as xo_budget.over became.
2. **Ownership** (who writes each field; both sides read):
   - ctx.xo: shield and unwinding, through the shields, xo_unwinding,
     and the budget poll.
   - budget.xo: the bud_* fields and the budget record (xo_budget, 96
     bytes, asserted).
   - fault.xo: faulting and the fault record.
   - The C: node, park, held/nheld/capheld (with the C Mutex code until
     B19), net_*, boundary, fault_exc, and the context's allocation and
     lifetime (task contexts live on the C stack in run_claimed; a root
     context is calloc'd by ctx()).
   - A move to Xo changes the owner in this list.
3. **Records C frees** (the budget record, its location text, the fault
   strings, held's array) are allocated by Xo at malloc's own address,
   `rt.heap_large(n) - 8` (gotchas/heap-large-prefix-not-malloc-address.md),
   and freed with `rt.heap_release` at that address.
4. **Generated code calls the Xo exports directly**:
   - the exports: `xo_x_ctx_shield_begin`, `_end`, `_abort`,
     `xo_x_ctx_unwinding`, `xo_x_budget_tick`, `_open`, `_end`;
   - selfhost/arm64 calls.xo maps the runtime call names;
   - selfhost/llvm call.xo `xo_export` does the same, passes pointer
     arguments as `ptrtoint` Ints, and emits no declaration for them,
     since they are defined in the module;
   - the C entry points forward to them for older xo's code (delete them
     after a seed).
5. **The current context in Xo**: `ctx.cur()` is thread local word 0. On
   a thread with no context it calls the C's ctx_root (address in
   rt.static_word 15), which makes a root context. The pool record's
   address is rt.static_word 16 (budget.xo cancels a scope's node
   through pool.xo).

6. **B18 (2026-10-14)**: the task and scope records follow the same rules
   (std/native/llrt/tasks.xo T_* and S_*, asserted in xo_rt.c; the C
   owns them, tasks.xo writes the result, the unwound flag, the fault
   strings, the failure fields, and the cancel node). The context gained
   `went` (C_WENT 216; xo_ctx is 256 bytes, 16 aligned), its own wait
   list entry, which the C's wl_wait_as and tasks.xo's wl_wait both use.
   rt.static_word 17 (ctx.xo's range) holds the program's task group.

## Consequences

- xo_rt.c: 287 lines deleted, 87 added (the asserts, the budget record,
  and one line forwarders). ctx.xo is 132 lines and budget.xo 341, with
  comments.
- XO-F005 (a shielded defer longer than 5 s) now prints its time as a
  Duration, as the Go backend does ("5.2s"), where the C printed %.3g
  seconds.
- held_check and held_del stay C: the C Mutex.with calls them on every
  with, and the call into Xo cost about 67 instructions per with on
  arm64 (bench/perf/ctx mutex 1.146). They move with the Mutexes (B19).
- Making a root context (ctx()) stays C: it counts the root in the
  deadlock record (dlk) and sets up the park, both the scheduler's (B18
  to B20).
- bench/perf/ctx on the Mac: 0.921 to 1.029 of main's instructions; the
  conc workloads are within noise (medians of 25 runs 0.80 to 1.012).

## Alternatives

- **Lower the shields as calls of native prelude functions in the SSA
  builder** (the `llrt_*` route): it would change the SSA, where
  rt.shield_end has its own landing rule (selfhost/ssa defer.xo), and
  wasm builds use the same builder. Mapping the runtime call names in
  the two native emitters keeps the SSA as it is.
- **Keep the per module offset copies**: B18 to B20 would add more
  copies of the same fields.
