# libunwind aborts when a forced unwind's stop function returns from _Unwind_Resume

Status: Observed (2026-10-14, decision 0221 stage 3, macOS arm64,
TestRuntimeFaultResume). Since decision 0221 point 10, macOS arm64 does
not use libunwind for faults; this still holds for x86_64 (libgcc and
libunwind) and for any future stop function.

Symptom: with the stop function of a fault's forced unwind moved to an
Xo export that returned a nonzero code at the context's boundary (so
that fault_raise would report the fault after `_Unwind_ForcedUnwind`
returned), the program aborted: `libunwind: _Unwind_Resume -
_Unwind_Resume() can't return`. This happened when a C frame with a
cleanup sat between the fault and the boundary (std/llrt/testdata/fault_resume.c).

Cause: the C cleanup resumes the unwind with `_Unwind_Resume`, which
continues the forced unwind from inside the cleanup's frame. When the
stop function then returns a code other than `_URC_NO_REASON`, the
forced unwind returns to `_Unwind_Resume`, which has nowhere to return
to and aborts. `_Unwind_ForcedUnwind`'s own caller only sees the return
when no cleanup resumed.

Fix: the stop function ends the program itself (fault_exit) and never
returns a stop code. On macOS the stop function stays in xo_rt.c
(fault_stop), and only the personality routine is Xo. Any future stop
function, in any language, must not return to stop.
