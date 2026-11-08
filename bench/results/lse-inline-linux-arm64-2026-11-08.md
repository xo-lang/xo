# Inline LSE atomics on linux/arm64: no gain (2026-11-08)

Question: on linux/arm64 the arm64 emitter lowers a read modify write
atomic (rt.slot_swap, rt.slot_add, rt.slot_cas) as a call of libgcc's
outline helper (`__aarch64_swp8_acq_rel` and the others; compiler/arm64
calls.xo atomic()), which tests `xo_lse` and runs the LSE instruction or
an ARMv8.0 load and store exclusive loop. darwin emits the LSE
instruction inline. Does inlining it on linux pay?

Experiment: branch lang1-lseexp (3fc311796, not pushed) emits darwin's
inline path on linux too (unsafe on ARMv8.0, a measurement only). In
bench/perf/metrics's linux/arm64 asm it removed all 772 `bl __aarch64_`
calls (266 ldadd, 284 cas, 223 swp inline).

Results: `xo bench --ab` on linux/arm64 host 1 (Graviton, 12 CPUs),
base 03001a2b3 (main), experiment 3fc311796; bench/perf/atomic does 2,000,000
operations per run.

| workload | rounds | A wall ms | B wall ms | B/A | interval | verdict |
|---|---|---|---|---|---|---|
| metrics counter | 7 | 118.8 | 118.9 | 1.0 | | within noise |
| metrics counterheld | 11 | 4.074 | 3.984 | 0.978 | 0.903 to 1.032 | within noise |
| atomic swap | 3 | 6.025 | 6.024 | 1.0 | 0.99 to 1.003 | within noise |
| atomic cas | 11 | 1.976 | 1.968 | 0.996 | 0.982 to 1.026 | within noise |
| atomic add | 3 | 5.995 | 6.033 | 1.006 | 1.0 to 1.013 | within noise (n=3 reads "worse" at 0.6%) |
| atomic contended | 3 | 13.14 | 13.02 | 0.991 | 0.984 to 1.113 | within noise |

(atomic cas at 3 rounds read 1.029 "worse"; 11 rounds put it at 0.996.)

Conclusion: the outline call costs nothing measurable. The helper's
`xo_lse` test and branch predict perfectly, and the call adds no spill
cost the bench can see. About 3 ns per operation either way. Not
pursued; the inline xo_lse dispatch design (nolibc links only, LL/SC
fallback, XO_LSE=0 to test it) is in the lead's notes should a
workload ever show the call in a profile.
