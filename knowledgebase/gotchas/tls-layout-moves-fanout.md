# One thread local more or less moves the fanout benchmark by 7 percent

Status: Observed (2026-10-09, while measuring decision 0085)

Symptom: `bench/perf/conc` fanout on arm64 measured 290 Mcycles with the
runtime of the contracts commit and 311 to 314 with the fault unwinding
runtime, in every bisection step, until the runtime of the commit before
the contracts (0d69fab) also measured 311: the faster runtime was the
outlier. The only runtime change of the contracts commit was one 8 byte
`static __thread` variable (`fault_detail_text`, removed again).

Cause: not proven. The thread locals the scheduler writes on every fiber
switch (`cur`, `tls_fib`, `tls_heap`) move with the layout of the thread
local block; the likely effect is which of them share a cache line, or
the alignment of the block dyld allocates per thread.

Fix: none yet. Measure native runtime changes against the parent commit
of the change, interleaved, and bisect the runtime separately from the
generated code (link the old `prog-darwin.s` with the new `xo_rt.c`).
Next experiment: align the scheduler's thread locals to 64 bytes each
(`__attribute__((aligned(64)))`) and measure spawn, fanout, and
pingpong; if the 7 percent comes back for good, keep it.
