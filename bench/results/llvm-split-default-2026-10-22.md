# LLVM builds: a size based split and memory aware clang jobs (2026-10-22, Xo Lang 2)

The default an LLVM build gets when XO_LLVM_SPLIT is not set: IR over
100 MB is split into modules of about 50 MB with thin LTO (smaller programs
stay one module with full LTO), and the clang jobs are the cores, no more
than three quarters of physical memory less xo's own heap at that point
fits at 3.5 GB a job. Mac with Apple silicon, 24 GB, macOS 27.0.1.

## A cold LLVM build of xo (IR 1,065 MB: 22 modules)

| build | clang jobs | wall | process group peak |
|---|---|---|---|
| one module (before) | 1 | killed by heavy.sh past 20 GB (22,251 MB) | over 20 GB |
| split, the new default | 3 | 1,234 s | 11,103 MB (ld 6.1 GB and xo 5.2 GB at the thin LTO link) |
| split, XO_LLVM_JOBS=4 | 4 | 1,695 s (other gates on the machine) | 8,687 MB |

xo's own counted heap peak (decision 0349): 8,469 MB at the IR's write.
Each clang of a module of about 50 MB peaked near 1 GB.

## The generated code: one module with full LTO (A) against a split of 8 with thin LTO (B)

`xo bench --ab`, LLVM `--release`, small programs forced to split
(XO_LLVM_SPLIT=8), so these bound the cost from above: the default does
not split them.

| program | metric | B / A | 95% interval | verdict |
|---|---|---|---|---|
| bench/perf/json parse | wall | 1.037 | 1.025 to 1.046 | worse |
| bench/perf/json parse | instructions | 1.073 | | worse |
| bench/perf/json small | wall | 0.933 | 0.847 to 1.152 | within noise |
| bench/perf/json small | instructions | 1.007 | | worse |
| CRUD server (bench/perf/http/xo, 256 connections) | rps | 0.991 | 0.92 to 1.039 | within noise |
| CRUD server | p99 | 1.062 | 0.91 to 1.182 | within noise |
| CRUD server | CPU a request | 1.016 | 0.9 to 1.076 | within noise |

Binary size: json 709 KB to 991 KB (+40%), the CRUD server 2.5 MB to 3.0
MB (+20%): thin LTO across modules inlines and drops less than full LTO
over one.
