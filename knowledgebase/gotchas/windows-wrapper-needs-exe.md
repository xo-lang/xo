# A command xo runs on Windows needs an extension

Status: Observed (2026-10-21, the xo-windows runner, CI runs 39654264421
and 39657249703)

## Symptom

Every LLVM golden of the Windows batch (scripts/windows-batch.sh step 2,
TestGoldenWant) failed with "the LLVM backend needs clang: install it or
set XO_CLANG", while `C:\Program Files\LLVM\bin\clang.exe` was on PATH
and a probe outside `go test` found it.

## Cause

internal/llvm's TestMain turns on the clang cache
(internal/xotool clangcache.go EnableTestClangCache): it builds a wrapper
and points `$XO_CLANG` at it. The wrapper was built as `.../clang`, with
no extension. xo looks `$XO_CLANG` up as exec.LookPath does on Windows
(compiler/xo tools.xo look_path_windows): a path without an extension is
tried only with each extension of %PATHEXT%, so `clang` alone was never
taken, and xo stopped before looking at PATH (an explicit `$XO_CLANG`
wins).

## Fix

The wrapper is `clang.exe` on Windows (exeName). Any file the tests hand
xo as a command on Windows needs `.exe`. compiler/xo tools_test.xo pins
the lookup's rules (PATHEXT set and unset, a path with and without an
extension, a file without one rejected), and windows-batch.sh step 2 runs
one golden through the same test first, so such a break is one error
instead of 271.
