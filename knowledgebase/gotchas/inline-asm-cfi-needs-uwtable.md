# An inline asm .cfi_escape needs a CFI region clang only sometimes emits

Status: Observed and fixed 2026-10-25 (the tasks code's `.cfi_escape 0x07,
30` in compiler/llvm/sched.xo and gen.xo's clone trampoline; queue item
"LLVM backend on Linux", TestGoldenTasks and TestRaceSyncTable failing on
the CI runner, main included).

## Symptom

On linux/arm64, clang rejects a naked function's inline asm that holds a
`.cfi_escape` with "this directive must appear between .cfi_startproc and
.cfi_endproc directives", so every LLVM test that compiles the whole
program's IR fails, and a plain `clang -c` of it does too. Wrapping the
asm in its own `.cfi_startproc`/`.cfi_endproc` fixes that path and then
breaks the others: all 12 `xo build` variants failed with "starting new
.cfi frame before finishing the previous one".

## Cause

Whether clang opens a CFI region around an IR level `naked` function
depends on the path: compiling a split part (xo's real builds) it emits
`.cfi_startproc` before the function's `//APP` asm, compiling the
whole-program IR (`xo asm`, which is what internal/llvm's tests compile)
it emits none, with or without `-fexceptions -funwind-tables`. A
function defined in C always gets one. The two forms of the directive are
therefore mutually exclusive across the two paths.

## Fix

Ask for the region in the IR: give the function the `uwtable` attribute
(`define void @xo_fiber_entry() naked noinline nounwind uwtable`). clang
then emits `.cfi_startproc`/`.cfi_endproc` in both paths, and the original
single `.cfi_escape 0x07, 30` (return address undefined, where a fiber's
or a clone child's backtrace ends) compiles everywhere. The attribute is
added only for linux/arm64, so darwin's IR is byte-identical to before.

Check both paths after any change here: `xo build --backend=llvm
--race` (split part) and `xo asm --backend=llvm` piped into `clang -c`
(whole program). Related: [[fault-position-abi]].
