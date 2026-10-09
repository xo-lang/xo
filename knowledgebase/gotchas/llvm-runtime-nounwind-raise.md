# A runtime function declared nounwind loses the landing pads around it (LLVM)

Status: Observed (2026-10-09, decision 0085)

Symptom: with fault unwinding on the LLVM backend, a fault in a small
function ran that function's deferred code but none of its callers': the
fault went straight to main's report. The IR had the `invoke` and the
landing pad in the caller.

Cause: the callee's unwind exit calls `xo_fault_continue`, which raises
the fault again. The runtime calls were declared `#2 = { nounwind }`.
With `-flto` LLVM inlined the callee into the `invoke` site, and a call
of a nounwind function inside the inlined body stays a plain call, so the
raise had no landing pad in the caller's frame. (Calling a nounwind
function that unwinds is undefined behavior in LLVM.)

Fix: runtime calls that can raise (`ssa.RuntimeRaises`: the faulting
runtime functions and `rt.fault_continue`) are declared with `#5`, not
nounwind; the fault functions (`#0`) are `noreturn cold` without
nounwind; generated functions are `uwtable`; the C runtime is compiled
with `-fexceptions -funwind-tables`. A new runtime function that can fault
must be added to `faultingRT` in `internal/ssa/landing.go`.
