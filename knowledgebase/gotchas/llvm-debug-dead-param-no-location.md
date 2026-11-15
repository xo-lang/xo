# An optimized LLVM build shows a parameter past its last use as unavailable

Status: Observed (Xo Lang 1, 2026-11-15, full tiers 76 and 77)

## Symptom
internal/arm64 TestDebugTypes/llvm failed with `lldb lacks "n = 2"`: at main.xo:19 in
the inlined `report`, `frame variable m` printed `<variable not available>`. The macho and
cc subtests passed.

## Cause
`xo build --backend=llvm` is optimized (lldb says "compiled with optimization"), and
report's last use of `m` is `m.len()` on line 18. Past it, clang owes `m` no location. It
had kept one only by how main's code fell out. c6a04be79 starts an interpolation with a
number from `rt.rs_with_cap`, and the test's `println("${report(...)} ...")` became one, so
clang ended `m`'s location before line 19. This is a dead value, not a lost live one.
Bisected over 5abcf6c6e..6e5bce7d2, and d4e8220cf (the isel flag) fails the same at its
parent.

## Fix
testdata/debugtypes calls `report` before the interpolation (3aad99e78). A test that reads
a variable from an optimized LLVM build should stop where the variable is still used, or
keep it live past the stop.
