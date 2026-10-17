# The C runtime cannot use constructors

Status: Observed (2026-10-14, B13 stage 2, CI run 38607314722).

## Symptom

Every program of TestDevGoldensELF failed to link on linux/arm64:
`xo: link: .../linux-arm64/rt.o: .init_array: constructors are not
supported`. The Mac's goldens and -short passed (the test is skipped off
Linux).

## Cause

A `__attribute__((constructor))` in std/llrt/xo_rt.c (setting obj.xo's
heap words before anything allocates) puts an entry in `.init_array`.
xo's own ELF linker (compiler/macho/elflink.xo), used when no cc links
the program, runs no constructors and rejects the section.

## Fix

Do the work at the start of the C `main` instead. For the C test
programs that bring their own main (`-DXO_RT_NO_MAIN`, internal/llvm's
TestRuntime* tests, linked by clang), a constructor under
`#ifdef XO_RT_NO_MAIN` is fine. Run CI (`suite=auto`) for any std/llrt
change: the ELF goldens only run on Linux.
