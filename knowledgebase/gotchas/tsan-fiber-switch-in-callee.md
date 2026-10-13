# ThreadSanitizer's fiber switch inside an instrumented callee breaks its shadow stack

Status: Observed (2026-10-13, decision 0212)

Symptom: with the fiber code in Xo (std/native/llrt/sched.xo), the
tasks.c stress test under ThreadSanitizer passed with one worker but
failed about 4 runs in 10 with three: `ThreadSanitizer: CHECK failed:
sanitizer_allocator_secondary.h:301 "((IsAligned(p, page_size_))) !=
(0)"`, or `DEADLYSIGNAL` and `nested bug in the same thread`, always
after `fanout: ok`.

Cause: the Xo code calls the C hook `xo_sched_hook`, which calls
`__tsan_switch_to_fiber` and returns. The C was compiled with
`-fsanitize=thread`, so the hook had `__tsan_func_entry` on the old
fiber's shadow stack and `__tsan_func_exit` on the new one's: every
switch popped another fiber's frame. The C path calls
`__tsan_switch_to_fiber` directly in `run_fiber` and `fiber_park`, whose
entries and exits pair up across the switch. (The Xo functions get
entry and exit calls too, as every function with calls does, even
without `sanitize_thread`; theirs pair up the same way.)

Fix: `xo_sched_hook` is `__attribute__((disable_sanitizer_instrumentation))`
(XO_NO_SANITIZE in xo_rt.c): no entry, exit, or access instrumentation.
A function that switches ThreadSanitizer's fiber must not be
instrumented unless its caller does the matching switch back before it
returns.
