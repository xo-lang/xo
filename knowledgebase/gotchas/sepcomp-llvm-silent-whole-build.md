# A failed build of std's LLVM objects measured as a perfect result

Status: Observed (2026-11-22, task #607, host 2).

Symptom: with `XO_SEPCOMP_LLVM=1` (decision 0738) and a compiler change under test, every benchmark
row came out at 1.00 of the whole program's time, where the rows had been at 1.15 to 1.3. The opt in's
binaries were the same size as the whole program's to the byte (bench/perf ctx 529,915 bytes both).

Cause: the change wrote IR clang rejected (an attribute group after an in-line `!dbg !DILocation(...)`),
so `xo dev stdobj --backend=llvm` failed, and the build fell back to the whole program, as designed.
It said so only in the `cache std llvm ...: not built (...)` line of `--phase-times`. A measurement
script that builds without that flag compared the whole program with itself.

Fix: a build that asked for the objects and could not make them prints
`xo: warning: std's LLVM objects were not built (WHY); the whole program is built (XO_SEPCOMP_LLVM=1)`
on standard error, whatever the flags (compiler/xo stdcache.xo std_variant_llvm; not when another
process holds the objects' lock, which is no fault). TestSepcompLLVM "objects not built are said".

How to avoid it: a measurement of an opt in checks that the opt in was taken (here: no such warning,
and a `cache std llvm ... (thinlto)` line under `--phase-times`) before it reads a number. Two builds
of the same size to the byte are one build.
