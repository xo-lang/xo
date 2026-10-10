# A runtime call that can fault must be listed in faultingRT

Status: Observed (2026-10-10, real-time sections, decision 0133)

Symptom: `task.isolate` caught XO-F007 (raised by `xo_realtime_exit`) on
arm64, but on LLVM the second isolated fault went straight to the top:
`fault: XO-F007 ... exit 70`.

Cause: the LLVM backend declares every `rt.*` runtime function
`nounwind` (attribute `#2`) unless `ssa.RuntimeRaises` says it raises,
which reads `faultingRT` in `internal/ssa/landing.go`. A forced unwind
through a call LLVM believes cannot unwind skips the landing pad, so the
isolate's handler never ran. arm64 has no such attribute and worked by
accident.

Fix: add the call to `faultingRT` (`"rt.realtime_exit": true`). Any new
C runtime function that can reach `fault_raise` needs the same entry, and
a test that catches its fault with `task.isolate` on LLVM.
