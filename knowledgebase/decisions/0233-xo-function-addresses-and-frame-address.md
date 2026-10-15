# 0233: Xo function addresses (rt.fn_addr) and frame address

Status: Accepted (owner, 2026-10-15). Step 1 implemented 2026-10-15
(xo-4, see Implementation): the intrinsics, with no use in a module
xo's own build reaches. Step 2 (the uses and the C deletions) waits for
seed 0.13.0.

## Context

The zero-C push (decision 0206, the audit in
bench/results/zero-c-audit-refresh-2026-10-13.md) moves std/llrt's C into
std/native. The inventory of xo_rt.c
(bench/results/zero-c-inventory-2026-10-15.md) finds that its largest
block of remaining C exists for one reason: **Xo cannot take the address
of a function**. C can, so C keeps:

- The services records (svc, pd_svc, stop_svc and their asserts). These
  are words of C addresses of cabi.xo exports, such as XO_FREE_FN of
  xo_x_task_free and xo_x_nc_conn_free. ctx.xo, poll.xo and stop.xo read
  them because they cannot name the exports' addresses themselves.
- The callbacks that poll.xo and stop.xo make through those records
  (pdsvc_*, stopsvc_*, poll_signal_cb, poll_stop_cb, stop_grace_fire).
  Most of them only forward back into Xo, and they exist because poll.xo
  and stop.xo sit below tasks.xo, chan.xo and twrun.xo in the imports.
- The signal handlers (on_signal; on_console and on_overflow on Windows).
  Any C ABI function can be a handler, and cabi.xo's exports are C ABI,
  but sigaction needs the handler's address.
- The scheduler services of rt.sched_rt that only forward back into Xo.
- The weak stand-ins in xo_http.c (xo_x_srv_free, hc_free, sink_free,
  cresp_free, streams_free). These exist so the services record can name
  std/http's exports in programs that do not link them.

A second, smaller block exists because Xo cannot read **its own frame
address**. run_entry and run_main set the context's unwinding boundary
to `__builtin_frame_address(0)` (decision 0085). The unwinder
(unwind.xo) never passes that frame. Everything else in those two
functions is calls into Xo.

The inventory's numbers for what this would free (xo_rt.c on main
275a0750):

| Inventory rank | Item | Lines |
|---|---|---|
| 3 | the services records, the poll and stop callbacks, the signal handlers, the scheduler forwards | about 300 |
| 5 | the handle code (h_init, h_release, h_free), once stop.xo's release calls tasks.h_release | about 60 |
| 7 | xo_worker and xo_fiber record types, after sched_hooks, fiber_map_failed and xo_sched_run_task move | about 100 |
| 9 | main as a short call into entry.xo | about 55 |
| 15 | run_entry's and run_main's boundary frame (the frame address) | about 15 |
| | xo_http.c's weak stand-ins | about 5 |

Ranks 5, 7 and 9 need rank 3 or rank 8 too (the inventory's "Needs"
column). Their lines count here because rt.fn_addr is their first
blocker. In all, about 535 lines, the largest single unlock left in
xo_rt.c (3,213 lines when counted).

## Decision (proposed)

Two std-internal intrinsics, declared in std/native/rt.xo next to
rt.fiber_start_addr (decision 0212), the existing intrinsic that already
yields a code address.

### 1. `rt.fn_addr(name) -> Int`: an Xo export's address

- **Semantics.** The argument names a top-level function of a C ABI
  export module: cabi.xo, cabi_*.xo, or any module whose functions the
  SSA builder declares as exports (selfhost/ssa build.xo
  declare_exports_of). The result is the address of its exported symbol
  `xo_x_<name>`, as an Int word. The draft took a reference to the
  function; the implementation takes its name as a string literal (see
  Implementation).
- **Calling it.** The address is called with the C calling convention
  through rt.call_c2 (decision 0207), or stored where C or the system
  expects a function pointer: the services record words, sigaction's
  sa_handler, and pthread_create's start routine through sys_c.xo
  (decision 0232).
- **Why only exports.** An export's signature is Int and Float
  parameters with at most one result (declare_exports_of rejects
  anything else), so its C ABI is defined. An ordinary Xo function's
  calling convention (closures, `var` parameters, multiple results,
  generics) is the compiler's own and may change. Its address would be
  a pointer nothing can call safely.
- **Errors.** Any other argument (not a string literal, or a name no
  export module defines) is a compile error from the SSA builder's
  unsupported path. It never reaches an emitter.

### 2. `rt.frame_addr() -> Int`: the calling function's frame address

- **Semantics.** The address C's `__builtin_frame_address(0)` gives: the
  frame record of the function that calls rt.frame_addr, which stays
  valid until that function returns.
- **Constraints.** The calling function always gets a frame (a frame
  record and frame pointer even when it would be a leaf), and it is
  never inlined. A caller's inlined copy would report the caller's
  frame and move the unwinding boundary.
- **First users.** A new run_entry in tasks.xo and run_main in
  entry.xo, which replace the C ones. Each stores rt.frame_addr() as the
  context's boundary (ctx.xo C_BOUNDARY, 112), then calls the entry
  function: the task's entry through rt.call_c2, or xo_entry.

### Safety

- Both intrinsics are std-internal like every rt function (decision
  0074). The checker resolves `rt` only inside std/native, so user code
  cannot name either one. Nothing in spec/core.md or spec/std.md
  changes, and no user program can observe a function address or a
  frame address.
- rt.fn_addr hands out only addresses of C ABI exports, which C already
  calls by name today. It adds no way to call Xo code that does not
  already exist.
- rt.frame_addr gives a stack address. It is used only as a bound that
  the unwinder compares frames against, as the C does now.

### Lowering

| Backend | rt.fn_addr(name) | rt.frame_addr() |
|---|---|---|
| arm64 (selfhost/arm64) | the symbol's address: `adrp` and `add` with :lo12:, through g.sym_addr, as rt.fiber_start_addr does for xo_fiber_entry (calls.xo); Mach-O and ELF relocations already exist for that path | the frame pointer x29 after the prologue; the function is marked to keep a frame (the arm64 frame decision, gotchas/arm64-call-anywhere-costs-frame.md) and never inlined |
| LLVM (selfhost/llvm) | `ptrtoint ptr @xo_x_<name> to i64`, as call.xo lowers rt.fiber_start_addr; the export is already declared or defined in the module | `call ptr @llvm.frameaddress.p0(i32 0)` then ptrtoint; the function gets `noinline` and `"frame-pointer"="all"` |
| Go backend (selfhost/gen) | not lowered: the Go backend runs std/rt (Go), not std/native, so no code it compiles can name either intrinsic; if one reaches it, it is an unsupported runtime call (a compile error) | same |
| wasm (selfhost/wasm) | a call that faults when run, in selfhost/wasm call.xo's native-only list (the place heap_large and the other native-only calls went, gotchas/wasm-rejects-calls-in-dead-native-arms.md): the wasm emitter emits every call of a reachable function, dead native arms included, and wasm has no C records to fill (its runtime is rt*.xo in selfhost/wasm) | same |

Both intrinsics join selfhost/ssa contract.xo's list of rt calls with
no contract (the list rt.fiber_start_addr and rt.stack_switch are in),
and the emitters' tests:
- test/codegen: the `adrp`/`add` of an export's symbol on arm64, and the
  `ptrtoint ptr @xo_x_` and `llvm.frameaddress` on LLVM;
- a golden that installs a signal handler from an export and stores an
  export in a services word;
- the existing unwinding goldens (fault_*, conc_*), which cover the
  boundary.

### Seeds

The seed builds xo from the checkout's std (decision 0216), so std-only
use does **not** avoid a seed. An older xo lowers an rt call it does not
know as a call of a C function `xo_fn_addr` or `xo_frame_addr`, which
nothing defines (gotchas/seed-older-than-intrinsics.md). Neither can
have the C shim that gotcha describes: C cannot know which export's
address is wanted, and a C function's frame address is not its caller's.

So the work lands in two steps:

1. The intrinsics in the checker's rt declarations, the SSA builder and
   both emitters, with codegen tests and a golden whose own code uses
   them. No module that xo's own build reaches uses them yet (ctx.xo,
   poll.xo, stop.xo, tasks.xo, entry.xo stay as they are), so the
   0.12.0 seed still builds xo, and the fixed point holds.
2. After the next seed (the owner's seed cycle: the lead cuts one when
   about 1.5K lines of seed-only C wait), ctx.xo, poll.xo, stop.xo,
   tasks.xo, entry.xo and hserve.xo use them, and the C above goes in
   batches of about 2K lines, each with an explicit seed build.

Step 1 is not a seed bump by itself. Step 2 waits for a seed the cycle
cuts anyway, so no extra seed and no chain hop.

## Implementation (step 1, 2026-10-15)

- **By name, not by reference.** `rt.fn_addr("name")` takes a string
  literal, as rt.sym_present does (decision 0232). The export modules
  import ctx.xo, poll.xo, tasks.xo and the rest, so the modules that
  need the addresses (ctx.xo init, poll.xo, stop.xo) cannot import
  cabi.xo to name `cabi.task_free` without a cycle. The name resolves to
  the symbol `xo_x_<name>` at link time, which no import needs.
- **Checked twice.** The SSA builder (selfhost/ssa call.xo fn_addr)
  requires a top-level function of that name in native/llrt/cabi or a
  native/llrt/cabi_* module. The exports themselves are declared only
  after the build's queue (declare_exports), so the builder cannot see
  the symbol yet. The verifier (verify.xo) then requires that some
  function of the module exports the symbol.
- **SSA.** A FuncAddr whose aux starts with `@` (`@xo_x_<name>`): a C
  symbol, not a function of the module. arm64 (isel.xo) and LLVM
  (func.xo) take that symbol's address; the passes that look a FuncAddr
  up by name fall back as for any unknown name.
- **rt.frame_addr.**
  - arm64: `mov $d0, x29` with Inst.frame, so the function is not a
    leaf (emit.xo) and the block has the frame (shrink.xo needs_frame0).
  - LLVM: `call ptr @llvm.frameaddress.p0(i32 0)`, and the function gets
    attributes #9 (`uwtable noinline "frame-pointer"="all"`); both are
    declared at the first use (sched.xo frame_attrs).
  - Both backends' SSA inliners (arm64 inline.xo, LLVM ranges.xo)
    treat a function that calls it as not inlinable. On LLVM this
    matters beyond the attribute: a call site marked #6 (alwaysinline)
    inlines a noinline callee
    (gotchas/llvm-callsite-alwaysinline-beats-noinline.md).
- **wasm.** Both are calls in selfhost/wasm call.xo's list that fault
  when run.
- **Go backend.** It runs std/rt, so it never compiles them. Its
  std/internal answers the test hook with what the native ops give
  (std/rt internal_crypto.go "rt").
- **Tests.**
  - The golden rt_addrs (internal/llvm/testdata/golden, every backend)
    goes through std/internal's crypto_op area "rt"
    (std/native/internal.xo rt_op). xo's own build calls only
    internal.lock_shared, so it never builds rt_op.
  - rt_op calls cabi.xo's fmt_int through its address, and compares
    rt.frame_addr with a callee's.
  - test/codegen/fnaddr checks the `adrp` of xo_x_fmt_int on arm64, the
    `ptrtoint ptr @xo_x_fmt_int` on LLVM, and the frame-keeping
    lowering of rt.frame_addr on both.
- **Gate (Mac, arm64).** selfhost/xo built by the 0.12.0 seed run
  directly (it links, so nothing xo builds reaches the intrinsics), hello
  on both backends, go test -short, rt_addrs on LLVM and arm64,
  codegen 6 of 6.

## Implementation (step 2, entry: 2026-10-15)

- tasks.xo run_entry stores rt.frame_addr in the context's C_BOUNDARY and
  calls the task's entry function through rt.call_c2; run_claimed calls
  it directly (rt.sched_rt op 29 and xo_rt.c run_entry are gone).
- entry.xo run_main (cabi.xo's run_main export is the main thread's C
  function, its argument xo_entry's address) sets main's root context,
  the boundary, and Windows' stack guarantee, then calls xo_entry.
- entry.xo main does what C main did after the heap's and the faults'
  setup (inventory rank 9). C main is one call of cabi.xo's entry_main
  after those two lines. The weak stand-ins of exports only some
  programs carry (cabi_http.xo streams_free, cabi_tw.xo tw_end) stay in
  C and are passed in by address. rt.fn_addr has no weak form, so it
  cannot name them.

## Consequences

- About 535 lines of xo_rt.c and xo_http.c can move. The services
  records become words that ctx.xo init fills from Xo, and the signal
  handlers become cabi.xo exports.
- run_entry and run_main leave the C. With rank 8 (the allocator entries
  and the main heap record) and rank 9, main becomes a short C function
  that calls an entry.xo export. It stays C only as the symbol the C
  library's start calls.
- Two more rt intrinsics for the emitters to keep. rt.fn_addr reuses the
  address path rt.fiber_start_addr already has. rt.frame_addr adds a
  frame-keeping rule on arm64 and a frame-pointer attribute on LLVM.
- Nothing user-visible changes, so the spec does not change.

## Alternatives

- **Leave the C (by design).** It keeps about 535 lines whose only job
  is to hand Xo a function address or a frame, and the services records
  stay duplicated between C structs and ctx.xo's SV_* constants with
  _Static_assert glue.
- **Fill the records from C, but generated.** The driver could emit a C
  table of export addresses. That is more C, not less, and a second
  table to keep in step with ctx.xo (gotchas/xo1-duplicated-tables-drift.md).
- **Restructure the imports so poll.xo and stop.xo call tasks.xo
  directly.** This removes some forwards (the inventory notes "a direct
  call where no import cycle stands in the way"). It does not help the
  services records, sigaction's handler or pthread_create's start
  routine, which need an address whatever the imports are.
- **Addresses of any Xo function.** Rejected: only exports have a
  defined C ABI (above).
- **A frame address from an assembly helper (sched.xo style).** A
  helper function's own frame is the wrong one. Reading the caller's
  saved frame pointer ties the result to the frame layout, which the
  intrinsic already owns on both emitters.
