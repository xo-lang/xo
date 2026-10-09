# AddressSanitizer warns when a fault unwinds on a fiber

Status: Observed (2026-10-09, decision 0085)

Symptom: TestGoldenASan failed on `http_fault` with an extra stderr line:
`WARNING: ASan is ignoring requested __asan_handle_no_return: stack type:
default top: ... size: 0x00005b8b7000`.

Cause: before a call of a noreturn function (the fault functions)
AddressSanitizer unpoisons the stack from the stack pointer to the top of
the stack it knows, the thread's. A handler runs on a fiber (a mapped
stack, decision 0069) that ASan was never told about, so the range was
1.5 GB and ASan refused.

Fix: the fiber switches call ASan's fiber API under `XO_ASAN`
(`__sanitizer_start_switch_fiber` before each `xo_fiber_switch`,
`__sanitizer_finish_switch_fiber` after it, as the ThreadSanitizer calls
next to them), so ASan knows the stack that runs. Any new place that
switches stacks needs both calls.
