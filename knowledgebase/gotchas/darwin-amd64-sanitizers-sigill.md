# Sanitizer binaries die with SIGILL on the Intel Mac

Status: Observed (2026-10-12)

Symptom: on the owner's Intel Mac (x86_64 i7-9750H, macOS 26.5.2, Apple
clang 17.0.0 clang-1700.3.19.1), every program built with
`-fsanitize=address` or `-fsanitize=thread`, even `int main(void) {
return 0; }`, exits 132 (SIGILL, "Illegal instruction: 4") before
`main`. The crash report puts it in `__pthread_init.cold.2` under
`libSystem_initializer`, while dyld runs the initializers of the
sanitizer dylib (`@rpath/libclang_rt.asan_osx_dynamic.dylib`). In
internal/llvm, TestCryptoDiff, TestJSONParseDiff, TestHTTPParseDiff,
TestHTTPFuzz, and every FuzzHTTPDiff seed failed with "signal: illegal
instruction" or "EOF" (their drivers are built with ASan). `-fsanitize=
undefined` alone runs. `ASAN_OPTIONS`, `MallocNanoZone=0`, and
`-mmacosx-version-min=13` do not help.

Cause: the sanitizer runtimes of that clang do not start on that macOS
release on x86-64 (a toolchain and OS mismatch, not an Xo bug: the
program never reaches the Xo runtime).

Fix: `asanFlags` in `internal/llvm/threads_test.go` probes once per test
binary (`sanitizerRuns("-fsanitize=address")`) and, when an ASan program
builds but cannot run, drops ASan from the flags
(`-fsanitize=address,undefined` keeps UBSan); the programs still run and
their output is still compared. ThreadSanitizer already had the same
probe (`tsanBuild`, gotchas/tsan-under-x86-emulation.md). Sanitizer
coverage on darwin/amd64 needs a newer Command Line Tools on that Mac
(the owner's call: no installs outside ~/xo-ci) or an llvm.org clang
unpacked into ~/xo-ci with `XO_CLANG` (next experiment).
