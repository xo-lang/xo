# Linux LLVM builds: a `use c` function inlined by LTO is not a frame of a crash report or traceback

Status: Observed (2026-10-30, TestCrashReport/llvm on the linux/arm64 offload host)

## Symptom

A crash report (or a fault's traceback) of an LLVM backend build on Linux
names the Xo caller where the faulting C function should be: the test
program's `crashc_write` (a store through a bad pointer) reported
`poke (main.xo:17)` first, no `crashc_write`. The same build on darwin
names the C function (`xo_c_crashc_<hash>_crashc_write`, then poke).

## Cause

The single module LLVM build links with full LTO (`-flto` at -O2,
split.xo single_lto), and on Linux the LTO link inlines the C function
and its `xo_c_` thunk into the Xo caller (objdump: the faulting store is
inside `x.poke`; .symtab has no `crashc_write`). ld64's LTO on darwin
keeps them out of line. The frame walker is right: there is no frame.
The C sources are compiled with no debug information, so the binary's
DWARF has one compile unit (the Xo one) and no `DW_TAG_inlined_subroutine`
for the C function: dwline's inline chain cannot show it as `[inlined]`
either.

## Fix

A C function that must stay a frame (a test of the walker, a function
whose name a report must carry) is marked `__attribute__((noinline))`,
as TestCrashReport's crashc.c is. Showing inlined C frames would need
the C sources compiled with `-g` (then the chain shows `name [inlined]`),
a separate item. A crash in inlined C code on Linux is reported at the
Xo caller's line.

Update (decision 0446, 2026-10-30): the C sources are now compiled to
objects with `-g` before the link (cffi.xo c_objects), so the report
shows an inlined C function as `name (file.c:LINE) [inlined]` above the
Xo caller, and an out of line one with its file and line; noinline is
needed only by a test of the frame walker itself.
