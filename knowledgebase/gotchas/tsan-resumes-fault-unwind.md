# ThreadSanitizer resumes a fault's unwind after its frame is gone

Status: Superseded (2026-10-14) by decision 0221 stage 3 and
[tsan-func-exit-pads.md](tsan-func-exit-pads.md): TSan builds no longer
add the cleanup pads, and on Linux arm64 the Xo unwinder passes C
frames over. The exception object still lives in `xo_ctx`, since
macOS's libunwind and glibc linux/amd64's libgcc still resume it from a
C cleanup. Observed (2026-10-09).

Symptom: on real Linux machines (EC2 c7g and c7i, Ubuntu 24.04, clang 18;
also Docker arm64 with clang 19) every golden that raises a fault failed
under ThreadSanitizer: TestGoldenThreads and TestGoldenTasks for
fault_unwind, fault_tasks, fault_audit, fault_task_exit, http_fault,
conc_reentrant, conc_send_closed, and others. TSan reported `SEGV on
unknown address` with pc 0 or a heap address, or `stack-overflow`, the
stack being `_Unwind_Resume` called from `xo_fault_raise` or
`fault_raise_msg`. Without TSan, with ASan, and on macOS the same
goldens passed, and the Docker validation of the morning ran before
fault unwinding (decision 0085) was merged.

Cause: `fault_raise` kept the `_Unwind_Exception` in its own stack frame,
assuming nothing resumes a fault (generated landing pads call
`xo_fault_continue`, which raises anew). TSan instruments every function
built with `-fexceptions`, C runtime and generated IR alike, with a
cleanup landing pad that calls `__tsan_func_exit` and then
`_Unwind_Resume`. That pad runs in a caller's frame, below which the
raising frames are dead; `_Unwind_Resume`'s own frame overwrote the
exception object, and it read its stop function (`private_1`) from the
garbage. Any C cleanup between the raise and the next generated frame
(`__attribute__((cleanup))`, C++ destructors) does the same.

Fix: the exception object lives in the context (`xo_ctx.fault_exc`), so
it outlives the raising frames. Regression test: TestRuntimeFaultResume
with `std/llrt/testdata/fault_resume.c`, a C frame whose cleanup scrubs
16 KiB of stack and resumes; it aborts with the old runtime on macOS and
Linux, built plainly and with TSan. Lesson: a native runtime change that
touches unwinding needs a Linux sanitizer run (`infra/ci-aws/ci.sh
sanitizers` or `scripts/linux-test.sh arm64`), because macOS TSan passed.
