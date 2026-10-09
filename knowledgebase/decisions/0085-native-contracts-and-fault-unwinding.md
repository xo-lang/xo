# 0085: native contracts and fault unwinding

Status: Accepted (2026-10-09). Supersedes point 4 of 0070 (a fault exits
without running deferred code) and the "a fault in a handler still exits"
item of 0083. Contracts: `internal/ssa/build_contract.go`,
`xo_fault_detail`. Unwinding: `internal/ssa/landing.go` (and the builder,
RC, and verifier hooks it names), `internal/llvm/fromssa.go` (`raise`,
landing pads), `internal/arm64` (`inst.pad`, call frame information and
call site tables in emit.go, `noCaller` in regalloc.go), the "Fault
unwinding" section of `std/llrt/xo_rt.c`, `task.isolate` natively, and
`dispatch` of `std/native/http.xo`. Goldens (Go, LLVM, and arm64
compared, leak check where the program goes on, ThreadSanitizer for the
task ones): `internal/llvm/testdata/golden/contract_*`, `fault_unwind`,
`fault_exit`, `fault_tasks`, `fault_task_exit`, `http_fault`.

## Context
The native backends (LLVM and arm64, decisions 0046 to 0084) lowered
`requires` but skipped `ensures` and struct invariants, so a program the
Go backend faults on printed a wrong result natively
(gotchas/native-ensures-unchecked.md), and hybrid builds (decision 0084)
had to exclude such functions. A fault exited the process at once
(decision 0070 point 4): no deferred code, no Mutex unlocked, a fault in
a task took the whole process down, `task.isolate` was not available,
and a fault in an HTTP handler killed the server instead of answering
500 (decision 0083). The Go backend unwinds a fault as a panic.

## Decision
1. **Contracts are checked in SSA where the Go backend checks them**
   (core 9.1, `internal/gen` fn.go and decl.go), with the same messages
   and locations:
   - `requires` at the start of the body (`requires failed: <source>`
     at the condition, or `Err(else)`).
   - `ensures` after the body against `result` and the `old(...)`
     values, which are taken after the `requires` clauses; a function
     that can fail checks them only on success. A function with
     `ensures` (or a `var` method of a type with an invariant) becomes a
     wrapper that calls its body as `<name>.body`, so every way out of
     the body (end, `return`, `?`) is checked once, after its deferred
     code, as on the Go backend.
   - A struct invariant after a struct literal (defaults included) and
     `with` (at that expression), and after a `var` method on success
     (at the method's name): `invariant of <Type> failed: <source>`,
     evaluated by `xo.invariant[T]` (one function per struct instance).
   - A failed `requires` or `ensures` prints the names it reads with
     their Debug text on the next line (`xo_fault_detail`), as std/rt's
     Require and Ensure do.
   Hybrid builds no longer exclude these functions (0084 point 1).
2. **Faults unwind with the platform unwinder, through landing pads
   that are the SSA's own unwind paths.** A fault (an operation's fault
   stub, `fault(...)` and failed contracts through `rt.fault_raise`, a
   runtime function that faults) records its message in the running
   context and calls `_Unwind_ForcedUnwind`. Every native frame has
   unwind tables: LLVM functions are `uwtable` and use `invoke`; the
   arm64 backend emits `.cfi_*` for every function and, for functions
   with landing pads, a GCC style call site table and
   `xo_personality` (a cleanup only personality routine in the
   runtime, used by both backends); the C runtime is compiled with
   `-fexceptions -funwind-tables`.
   - A landing pad enters the cleanup successor of an SSA check, the
     same code cancellation runs (decision 0068): the deferred code of
     the blocks being left (shielded), scope closes (canceling and
     waiting for their tasks), the unlock of a `with`'s Mutex, the
     releases InsertRC put on that edge, then the function's unwind exit
     (`Func.UnwindExit`), which raises the fault again
     (`rt.fault_continue`) so it reaches the caller's pad.
   - A value that can fault carries its landing (`Value.Pad`, the
     check's call). Checks are placed (a) after calls, where the builder
     already put cancellation checks; a check only a fault takes is
     `rt.landing`, which backends emit as nothing (LLVM: `br i1 false`;
     arm64: an `iJump` whose second target is the pad, for liveness and
     reachability); (b) by the builder after an operation that can fault
     inside a block with deferred code or an open scope, with the open
     blocks and the variables as they were at the operation
     (gotchas/landing-reads-later-definitions.md); (c) after the build,
     after every other operation that can fault. After InsertRC the
     counting code between an operation and its check moves into the
     check's successors (the operation's own results do not exist on
     the cleanup path; InsertRC and the RC verifier treat them as not
     held, and a canceled call's zero values own nothing), and checks
     whose cleanup does nothing are removed: such a fault passes through
     the frame. The RC verifier requires that nothing be held at an
     operation that can fault without a landing.
   - **The no-fault path pays nothing at run time**: no flag test after
     calls and no per-frame registration; checks that only a fault takes
     cost no instruction. What remains is code size (pads, tables) and,
     on arm64, that a value a pad reads lives in a callee saved register
     (it must survive the call; a fault stub's call is not a call for
     the allocator, so those values are marked `noCaller`).
   - **Catching.** A task's entry function lands its closure call:
     `rt.task_unwound` moves the fault to the task, and its scope treats
     it like a failed child (cancels the body and the other tasks); the
     scope's close returns status 3 and the parent raises the task's
     fault there (`rt.fault_rethrow`). `task.isolate` lands its call and
     turns a fault into `Err(Isolated.Faulted(info))` (`rt.fault_take`;
     cancellation goes on unwinding). Past the frame that runs main
     (`boundary`, or the end of the stack) the runtime prints
     `fault: <msg> at <loc>` (and the detail line) and exits with 70.
   - **Mutex.with with a `var` closure**: the closure (`Func.Catches`)
     returns on a fault as on cancellation, handing back the value it
     was given, so the mutex keeps its old value (as std/rt writes back
     only on success) and is unlocked by the caller, which then unwinds.
   - **HTTP**: `dispatch` runs each handler in `task.isolate`; a fault
     logs `handler fault` (field `fault`: message and location) at error
     level and answers 500 `{"error":"internal error"}`, and the server
     keeps serving.
3. **Mechanism choice.** Table based unwinding (libunwind) over
   setjmp/longjmp per task frame: longjmp skips the frames, so it cannot
   run their deferred code or release what they own (the reason 0070
   gave), and a jump buffer per frame costs on every call. Explicit
   flag tests after every call (cancellation's mechanism, decision 0068)
   would cost a load and a branch per call on the no-fault path.
4. **Process.** A fault that unwinds stays recorded on its context;
   deferred code that faults again replaces it (as a panic in Go's
   deferred code). The exception object lives in the raising frame (a
   landing pad never resumes it). AddressSanitizer learns the fiber
   switches (gotchas/asan-fiber-no-return.md).

## Consequences
- The goldens above print the same on the Go, LLVM, and arm64 backends;
  where the program goes on (`task.isolate`, the server) the leak check
  counts no allocation live, also under ThreadSanitizer with 1, 2, and N
  workers.
- Benchmarks (2026-10-09, bench/perf/llvm/native.sh 11 runs and
  bench/perf/conc 9 runs, against 0d69fab): LLVM unchanged; arm64 within
  noise except sort 216.9 -> 221.1 ms (+1.9%: the `var` list parameter
  lives in a callee saved register for its pads) and strings +1.4%;
  conc workloads within noise. While measuring, a thread local layout
  effect moved fanout by 7% (gotchas/tls-layout-moves-fanout.md).
- `Mutex.with` closures with a `var` heap value that can fault after
  changing it copy the value first (the old value must survive for the
  mutex); before, they updated it in place.
- Native fault reports still lack frames and crash bundles, and
  `Fault.frames` is empty (core 8, 11.2).
- A fault inside deferred code is unwound without the rest of that
  frame's cleanup (its landing is the function's, not the deferred
  code's); the Go backend runs the remaining deferred code.
- A fault in a server's connection task outside the handler (std code
  reading the request) fails the server's connection scope instead of
  ending only that connection.
- `xo build --hybrid` builds SSA with `NoFaultUnwind` (Go mode assembly
  faults by Go panic).

## Alternatives
- **Check `ensures` at every `return` in the builder**: each exit would
  need its own copy of the clauses, and deferred code runs after the
  return value is computed; the wrapper gets the order right in one
  place.
- **setjmp/longjmp per task frame** (the next experiment 0083 named):
  cheap to build, but leaks what the skipped frames own and skips their
  deferred code; rejected above.
- **Explicit unwinding through flag tests** (cancellation's way):
  correct without unwind tables but costs every call on the no-fault
  path.
- **An SSA edge for every operation that can fault**: splits blocks in
  hot loops and changes what the arm64 backend's loop optimizations see;
  only the checks that have cleanup to do are kept, and they cost no
  instruction.
