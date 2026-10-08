# The first run of a freshly built binary is slow on macOS

Status: Observed (2026-10-08, bench/perf/llvm programs)

Symptom: a hello world Xo binary took 0.21 s on its first run and a few
milliseconds after; the sieve measured 0.55 s once and 0.14 s on every
later run, with 36 percent CPU use on the first.

Cause: macOS checks a new executable (code signature and policy) on its
first launch; the process waits without using the CPU.

Fix: time best of N runs (`bench/perf/run.sh`, `bench/perf/llvm/run.sh`
do), or run a new binary once before timing it. A single `time ./bin`
right after `xo build` is not a measurement.
