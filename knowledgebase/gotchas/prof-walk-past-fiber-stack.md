# A profiler sample walked past the top of a fiber's stack into the next one's guard page

Observed 2026-11-05.

Symptom: a program run with `XO_PROF=cpu` (here at 1,000 Hz) dies with
`fatal error: SIGSEGV: segmentation violation (code 2) at address
0x...008`, traceback `cabi.on_prof` and an unrelated name, in some runs
only: 8 of 30 runs of 20 x spawn100k on linux/arm64, `--release
--profile-support`; 1 of 30 with 0.39.0's xo. darwin/arm64 did not show it
(0 of 30; its compact unwind path checks frame records differently).

Cause: unwind.xo's walk_regs bounded each frame's CFA only by "at most
PROF_SPAN (64 MiB) above sp", not by the stack's end. Fibers' stacks are
mapped next to each other, each with its guard page at the bottom, so the
page past one fiber's top is often the next fiber's guard. A sample whose
frame rule put a saved register past the top (the outermost frame on a
fiber) read the guard (SEGV_ACCERR, the address a page plus 8) inside the
signal handler.

Fix: prof_walk takes the interrupted thread's fiber (TLS_FIB); when sp lies
in that fiber's mapping, walk_regs and step read nothing at or past its end
(as Go bounds a traceback by the goroutine's stack). Test:
internal/llvm TestProfSpawnStress (16 runs of the program that showed it;
it failed twice in two runs without the fix). A sample's frame layout
decides whether a rule meets the edge, so a smaller or debug build of the
same work passed 16 runs without the fix: keep the test's program as it is.
The darwin sampler thread (prof.xo mac_sample) cannot read the target
thread's TLS and passes no fiber, so it keeps the span bound.
