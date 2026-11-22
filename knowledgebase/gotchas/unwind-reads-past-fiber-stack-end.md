# The fault walks read the word past a fiber's stack

Status: Observed (2026-11-21, Xo Lang 1, task #650, darwin/amd64).

Symptom: on the Intel Mac every fault of a default (debug) LLVM build
died with `fatal error: SIGSEGV: segmentation violation (code 1) at
address 0x155510000`, a page aligned address just above the stack
pointer, with one frame and no traceback, where the program should
have printed its fault and exited 70 (cmd/xo TestBuildRunLLVM,
TestHolesNative, TestFieldEnsuresResult, TestTracebackIsolatedNoLeak at
v0.55.0). `--release` LLVM builds and the native backend were right, and
so was every other platform.

Cause: a fiber's outermost frame (`xo_fiber_entry`) is entered with
nothing above it on its stack. The walks of the calling thread in
std/native/llrt/unwind.xo (`raise`, `trace_frames`, `trace`) called
`step` with the bounds `0, Int.max`, so the DWARF rule "the return
address is at CFA - 8" for that frame read the word at the mapping's
end. On darwin/amd64 nothing is mapped there. Elsewhere the read
happened to succeed (another mapping follows the stack) or the frame
was stepped by its compact encoding, and the walk ended at the next
lookup, so the bug was silent. prof_walk had the bound from the start.

Fix: `stack_end(sp)` gives the running fiber's mapping's end when sp
lies in it, the three walks pass it to `step`, and a saved register at
or past the end ends the walk as a pc of 0 does.

How to avoid: a walk that reads the stack takes the stack's bounds; a
caller passing `Int.max` as a bound is reading whatever follows the
stack. A crash address that is page aligned and just above `rsp` or
`sp` is a read past a stack's end, not a wild pointer.
