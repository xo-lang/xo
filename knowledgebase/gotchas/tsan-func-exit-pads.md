# ThreadSanitizer's function exit pads resume through libgcc

Status: Observed (2026-10-14, decision 0221 stage 3, read before
implementing and checked with clang on the Mac)

Symptom (what would happen): with the Xo fault unwinder on Linux, a
program built with `-fsanitize=thread` (internal/llvm tsanBuild:
TestGoldenTasks, TestRuntimeThreads, TestRuntimeFaultResume) crashes on
its first fault, or TSan's shadow stack drifts and reports nonsense.

Cause: TSan instruments function entry and exit. In every function that
makes a call it adds a cleanup landing pad that calls `__tsan_func_exit`
and then resumes the unwind (`resume`, so `_Unwind_Resume`). In C the pad
sits under `__gcc_personality_v0`; in generated IR it goes into the
function's own call site table under `xo_personality`. The Xo unwinder
(std/native/llrt/unwind.xo) would enter the pad of a generated frame, and
`_Unwind_Resume` (libgcc) would then call xo_personality with a libgcc
context it cannot read. It passes C frames over, so their
`__tsan_func_exit` would never run.

Fix: tsanBuild compiles with `-mllvm -tsan-instrument-func-entry-exit=0`.
That adds no pads at all (0 landingpad, `__tsan_func_*`, or `resume`
lines in `clang -S -emit-llvm` of a C file and of a .ll whose function
has `personality ptr @xo_personality`). Race detection is unchanged;
reports lose the call stack above the racing frame. Builds through
`xo build` have no sanitizer mode, so only the tests are affected.
