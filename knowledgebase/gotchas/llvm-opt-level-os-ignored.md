# XO_LLVM_OPT=Os is ignored without a word

Status: Observed (2026-10-19).

Symptom: an LLVM build of xo with `XO_LLVM_OPT=Os` took 15 minutes and
produced a `__text` byte for byte the size of the -O2 build.

Cause: compiler/xo split.xo accepts only `O0`, `O1`, `O2` and `O3`; any
other value leaves the default (-O2, or O0 for `xo test`) and nothing
says so.

Fix: use one of the four levels. Measuring size against speed with
clang's `-Os` or `-Oz` needs those values added to split.xo first.
