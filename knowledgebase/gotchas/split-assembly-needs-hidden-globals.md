# A split arm64 assembly failed to link: a symbol local to one file

Status: Observed (2026-10-14), fixed (e9b0470a).

Symptom: the first parallel generation whose parts assemble apart
(commit 0653188b) built every part, then cc's link failed with
`Undefined symbols for architecture arm64: "_xo_fiber_entry",
referenced from: _xo_f_ssa_2eBuilder_2ewith__closure in
prog-darwin.31.o`. Before it, ld warned for every part object `was
built for newer 'macOS' version (27.0) than being linked`.

Cause: a file of a split assembly can only reach another file's labels
through global symbols. The functions, the system call trampolines, and
the helpers were made hidden globals, but sched.xo's `xo_fiber_entry`
(the first frame of a new fiber, which `rt.fiber_start_addr` takes the
address of) was a plain local label in the last file. The warnings: the
parts were assembled by `cc -c` without the `-isysroot` the link uses,
so their objects carried the host's OS version.

Fix: `sched_asm(os, split)` marks the fiber entry `.globl` plus
`.private_extern` (Mach-O) or `.hidden` (ELF) in a split generation; the
parts are assembled with the SDK of cc_build's sysroot memo
(asmparts.xo `memo_sysroot`). Anything the emitter writes once per
program and the parts reference (trampolines, task switch code, data
words) must go through `G.tramp` or a hidden global the same way.
