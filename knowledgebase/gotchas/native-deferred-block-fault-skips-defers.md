# A fault inside deferred code past a pattern or a nested block skipped the remaining defers (native backends)

Status: Observed and fixed (Xo Lang 2, 2026-10-17).

## Symptom

On LLVM and arm64, a fault in deferred code skipped the deferred code
registered before it, and the blocks around it. With tasks, a program
could end in `fault: XO-F001: deadlock: every task is blocked`. The Go
backend ran every remaining defer, as core 5.6 requires. Shapes:

- `defer { let y = f() }`, or a `for` over a list, Bytes, a Map, or a
  channel inside a deferred block, whose element call faults;
- deferred code of a block that is itself inside deferred code
  (`defer { defer { boom() } }`), and deferred blocks in loops inside
  deferred code.

`defer f()` and `defer { f() }` were right. This was found by rewriting
fault_defer_chain's `defer boom(...)` as `defer { let _ = boom(...) }`
while adding XO0209 (decision 0258). progen.DeferFaultProgram then
showed the native backends differing from Go in 14 of 20 programs.

## Cause (compiler/ssa)

Three gaps in how the SSA builder gives operations in deferred code
their landing (decision 0085):

1. In deferred code check_then leaves a call's landing to flush_landing
   at the next instruction. A `let` (and the `for` loops' element
   patterns) switched to the pattern's fail block first, so the flush
   ran there and dropped the landing. add_landings then sent the fault
   to the unwind exit.
2. Only top level deferred code got fault continuations
   (defer_landing_here required shield 1). Deferred code of a block
   inside deferred code had none.
3. flush_landing ran the deferred code of the blocks opened inside the
   faulting statement with no landing set. My first fix also ran them at
   shield 0 from fault_cont's new path, where they counted as top level.
   Either way their faults unwound past the statement.

## Fix

- stmt.xo end_fail flushes the pending landing before a pattern's fail
  block, at all four sites (let, for over lists and Bytes, for over a
  channel, for over a Map).
- A block whose deferred code runs inside deferred code records that
  statement (DFrame.up, up_at), and its statements get continuations.
  At index 0 its continuation runs the blocks between and continues the
  statement's fault. DLand.lvl replaces the fixed shield 1.
- flush_landing and fault_cont run the inner blocks' deferred code with
  the statement's landing, at its level.

Tests: the goldens fault_defer_block (fault_defer_chain in block form)
and fault_defer_shapes on every backend, and TestDeferFaultProperty
(internal/gen, Go against native).
