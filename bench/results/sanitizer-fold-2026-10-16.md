# The run-time sanitizer checks' cost (ADR 0232 follow-up), 2026-10-16

Question: would a build-time `target.sanitizer` constant, folding
std/native/llrt/sanhook.xo's checks away, beat the run-time
rt.sym_present checks (a GOT load and compare in sanhook.tsan, asan,
sanitized, and on every release/acquire and switch hook)?

Method: bench/perf/conc's six workloads, each built twice by xo (main
8b90e631-era, --ld=cc) on arm64 and llvm: with the current std, and with
a copy whose sanhook.tsan, asan, and sanitized return false (the best a
constant could do). `/usr/bin/time -l` instructions retired and cycles,
interleaved rounds, Mac (darwin/arm64). Scripts kept out of the repo per
the repro script rule.

Threaded (default XO_THREADS, 7 rounds): within run to run noise, as the
workloads spin under contention (the 15 round rerun of mutex and
rwmutex: minimums 120.0 vs 124.8 M instructions for mutex-arm64, the
fold the slower).

One thread (XO_THREADS=1, 5 rounds; folded / current, medians):

| workload | instructions arm64 | instructions llvm | cycles arm64 | cycles llvm |
|---|---|---|---|---|
| fanout | 0.997 | 0.996 | 0.932 | 1.095 |
| mutex | 1.017 | 1.013 | 1.066 | 0.969 |
| pingpong | 0.995 | 0.952 | 0.989 | 0.901 |
| pmap | 1.000 | 1.000 | 0.999 | 1.017 |
| rwmutex | 0.996 | 0.978 | 1.007 | 0.991 |
| spawn | 1.001 | 0.995 | 0.961 | 0.972 |

(pingpong-llvm by minimums: 288.7 to 280.3 M, 0.971; rwmutex-llvm 305.4
to 299.7 M, 0.981.)

Result: no gain on arm64; at most about 3% fewer instructions on two
llvm workloads (pingpong, rwmutex), cycles within noise. Not worth a
build-time constant, which needs a new xo flag that must match clang's
-fsanitize (a mismatch would drop the sanitizer hooks silently): the
run-time check stays (ADR 0232).
