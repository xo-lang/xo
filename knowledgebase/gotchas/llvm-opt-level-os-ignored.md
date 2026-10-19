# XO_LLVM_OPT=Os is ignored without a word

Status: Observed (2026-10-19), fixed the same day: any other value is now an error
(`xo: XO_LLVM_OPT=Os: the level is O0, O1, O2, or O3`).

Symptom: an LLVM build of xo with `XO_LLVM_OPT=Os` took 15 minutes and
produced a `__text` byte for byte the size of the -O2 build.

Cause: compiler/xo split.xo accepts only `O0`, `O1`, `O2` and `O3`; any
other value leaves the default (-O2, or O0 for `xo test`) and nothing
says so.

Fix: compiler/xo split.xo rejects the value and the build stops. Use one of the four levels. Measuring size against speed with
clang's `-Os` or `-Oz` needs those values added to split.xo first.
