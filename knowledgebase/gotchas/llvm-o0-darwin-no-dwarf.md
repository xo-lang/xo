# An LLVM -O0 debug build on darwin has no source lines

Status: Observed (2026-11-15; 0.51.1 too, so not a 0.52 regression)

## Symptom

`xo build --backend=llvm --opt=O0` on darwin: lldb cannot stop at
`main.xo:LINE`, the dSYM has no compile units. -O1 and LTO builds are fine.
Linux is not affected (host 2, 0.49.1 seed: main.xo rows at -O0).

## Cause

Without LTO the single module path gave clang the `.ll` in its one step
link. clang compiled it to a temporary object in TMPDIR, linked, and
deleted it; the debug map (N_OSO) named only that object, so dsymutil
found no DWARF.

## Fix

compiler/xo link.xo `ir_objects` compiles each `.ll` to a kept object
before the link when a dSYM is made and LTO is off. Checks:
internal/llvm TestDebugLinesLLVM (darwin and linux, reads the line table
with debug/dwarf) and internal/arm64 TestDebugTypes `llvm-O0` (lldb).
