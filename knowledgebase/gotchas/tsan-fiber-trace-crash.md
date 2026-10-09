# ThreadSanitizer crashes on Darwin after many fiber tasks

Status: Observed (2026-10-09, std/llrt task runtime, decisions 0069, 0077)

Symptom: a native program built with `-fsanitize=thread` (as
TestGoldenTasks builds the goldens) prints only
`ThreadSanitizer:DEADLYSIGNAL`, sometimes `BUS on unknown address` in
`alloc_mt` or `xo_rl_new` with a stack pointer near 0x7040000000, or hangs
with a thread in `__tsan::TraceSwitchPartImpl` waiting on an unfair lock.
It happened with `XO_THREADS=1` or `2`, not with one worker per CPU, for
the spawn benchmark at 200,000 tasks (fan out at 100,000) after decision
0077; the same program at 2,000,000 tasks crashes the runtime from before
0077 too. 20,000 tasks pass. The same binaries without the sanitizer, at
-O1 and -O2, run clean with zero leaks.

Cause: inside ThreadSanitizer (the fault is in its trace part switching,
not in a report about the program); it grows with the number of
instrumented events across fiber switches. Not narrowed further: routing
the runtime's futex waits through condition variables ThreadSanitizer
intercepts did not change it, and a free list canary build found no heap
corruption.

Fix (workaround): run ThreadSanitizer on reduced sizes: the goldens and
`std/llrt/testdata/*.c` are well below the limit, and the conc benchmarks
were checked under it at a tenth of their size (`bench/perf/conc`,
results file). Next experiment: build the sanitizer runtime of a newer
LLVM (or Linux) and see whether the crash follows the Darwin runtime;
`TSAN_OPTIONS=verbosity=2` to see whether a reset (`DoReset`) precedes it.
