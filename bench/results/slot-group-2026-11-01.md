# Spill slots of a word run together, 2026-11-01

Pass slot_group (compiler/arm64 regalloc.xo), native arm64 only. Size:
xo at lang1-slotgroup on ebfb1e308, darwin/arm64 on the Mac, builds
compiler/xo with the pass on and off (`XO_PASSES=-slot_group`). Last
verified: 2026-11-01.

## Size

| | off | on | |
|---|---|---|---|
| xo's __text, bytes | 36,521,044 | 35,630,348 | -2.4% |
| copy runs (ldr/str runs of 8 or more), bytes | 2,547,096 | 1,634,696 | -35.8% |
| frames, total bytes | 5,311,712 | 5,458,480 | +2.8% |
| largest frame, bytes | 25,776 | 28,512 | none over 32K |

Before the held merge (ebfb1e308), on d9fdb4a6f, the same comparison was
38,087,604 to 36,526,412 (-4.1%): the two changes save some of the same
copies.

## Speed

`--ab`, linux/arm64 host, 3 rounds, measured before the rebase
(d9fdb4a6f against f7128084c):

| Program | wall | verdict |
|---|---|---|
| self | 0.998 (0.994 to 1.011) | within noise |
| self max RSS | 1.004 (1.003 to 1.007) | worse |
| bench/perf/fault owned | 0.993 (0.987 to 1.016) | within noise |
| bench/perf/fault defer | 0.983 (0.982 to 1.001) | within noise |
| bench/perf/alloc mixed | 0.991 (0.989 to 0.997) | better |
| bench/perf/alloc strings | 0.973 (0.94 to 0.986) | better |

Identical-code controls on that host read wall 1.004 and 1.001, max RSS
1.007 and 1.006 (2026-11-01, infrastructure/offload.md): the verdicts for
self max RSS and alloc mixed above are inside that band, so they are no
claim.

HTTP: bench/perf/http/run.sh, linux/arm64, arm64 backend, medians of 5
runs per round, 32 workers, 10 s after 2 s warmup, loopback, ebfb1e308
against the change on it. No errors and no GCs in any run.

| Round | req/s | ratio | CPU us/req | p99 us |
|---|---|---|---|---|
| 1 | 186,630 to 187,711 | 1.006 | 11.84 to 11.73 | 835 to 831 |
| 2 | 186,220 to 186,528 | 1.002 | 11.86 to 11.79 | 839 to 842 |
| 3 | 185,886 to 187,837 | 1.010 | 11.83 to 11.71 | 839 to 835 |
