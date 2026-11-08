# A frame closure test in one place only leaks its heap environment

Status: Observed (2026-11-07)

**Symptom.** After letting a closure's environment pass 32760 bytes (the
wide_capture golden), the arm64 golden printed the right output but
failed "1 allocations still live at exit": a plain closure that was only
called leaked its environment; the fault frame closure did not.

**Cause.** compiler/arm64 decides "environment in the frame" in
stack_closures, and two places read it: closure_op (where the
environment goes) and the release lowering in isel.xo (a frame closure's
release only releases its captures, stack_env_release, and never frees).
The first fix put the size test in closure_op only, so the wide closure
got a heap environment while its release still took the frame path.

**Fix.** Put any condition on frame placement in stack_closures itself
(it now leaves out environments past 32760 bytes), so every reader
agrees. Check with XO_LEAKCHECK=1 on a program that only calls the
closure, not only on one that stores it.
