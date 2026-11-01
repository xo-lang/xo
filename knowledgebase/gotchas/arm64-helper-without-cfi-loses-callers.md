# An arm64 helper with no call frame information loses its samples' callers

Status: Observed (2026-11-01).

**Symptom.** In a CPU profile (`XO_PROF=cpu`) a helper's flat time is shown,
but `go tool pprof -peek` lists no callers, and the callers' cumulative time
leaves it out. The retain thunk read 1.78% of a server's samples with an empty
caller list.

**Cause.** The profiler walks frames by the unwind tables, and the helper had
no `.cfi_startproc`, so the walk stops at the leaf. A frameless helper needs
only the empty pair: with no directives the return address is in x30.

**Fix.** Put `.cfi_startproc` after the helper's label and `.cfi_endproc` at
its end, with `.cfi_adjust_cfa_offset` around any stack change (rc.xo
inc_thunk). Check with `go tool pprof -symbolize=none -peek NAME FILE.pprof`
on a run long enough to sample it (a short `bench/perf/alloc mixed` run took
118 samples and none in the thunk; the HTTP server under load works).
