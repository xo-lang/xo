# A small separately compiled program regenerated all its code on every build

Status: Observed (2026-11-10, Xo Lang 2)

## Symptom
coll's warm build (unchanged source, XO_BIN_CACHE=off) spent 55 to 58 ms of about 150 in emit with
every pass served from its cache; `--phase-times` printed no `fn_keys` lap and no "cache parts" line,
while XO_SEPCOMP=0 showed "cache parts 14/14 reused" and emit 5 ms.

## Cause
compiler/arm64 function_keys and part_keys return no keys for a module of one part (fewer than
PART_FUNCS functions: "generated whole, not cached"). A separately compiled program (decision 0388,
the default) takes generate_cached's parts path even as one part, but with no keys nothing was looked
up or stored.

## Fix
Both functions key a separately compiled program of one part (m.sepcomp_program). Warm coll on
linux/arm64: wall 0.479 (bench/results/perf-gaps-2026-11-10.md). When a phase is slow with the caches
warm, look for the cache's lap (`fn_keys`) and hit line in `--phase-times` before profiling the phase.
