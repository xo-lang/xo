# Node (TypeScript) against Xo: 2026-11-22 (HTTP rows)

Written by bench/perf/node/run.sh (twins/lib.sh). Ratios are Xo over Node (TypeScript) (below 1: Xo is faster or smaller).

## Machine and versions

- Linux 7.0.14-linuxkit aarch64, host 1 (the linux/arm64 timing host, exclusive lease), 12 CPUs, load average at start 3.77
- xo 0.55.0 commit 9599fc7d key ff3b0011426efd26, NOT THIS CHECKOUT (its key is )
- go version go1.26.11 linux/arm64
- node v20.19.2, typescript 5.9.3 (dist/ compiled on another host; no npm here), @types/node and typescript pinned in package-lock.json


## HTTP: the users CRUD API (node:http, one process; Go net/http; Xo std/http)

```
# config rps p50_us p99_us p999_us errors gcs pause_ms peak_heap_mb gc_cpu_pct peak_rss_mb gcs_per_10k cpu_us_req
go 122913 187 1192 2671 0 2903 680.01 3 4 19.5 19.87 27.60
xo-arenas 179282 142 926 2530 0 0 0.00 0 0 13.0 0.00 11.67
node 87007 330 868 1280 0 0 0.00 0 0 92.6 0.00 11.49
go 123605 186 1178 2610 0 2837 648.89 4 3 19.3 19.14 27.13
xo-arenas 178309 141 921 2431 0 0 0.00 0 0 14.4 0.00 12.12
node 83685 344 920 1356 0 0 0.00 0 0 92.8 0.00 11.94
go 122531 188 1204 2698 0 3033 703.85 3 4 20.5 20.58 27.67
xo-arenas 179334 142 920 2365 0 0 0.00 0 0 15.0 0.00 11.79
node 70662 355 1503 2409 0 0 0.00 0 0 92.2 0.00 14.21

Medians of 3 runs, 32 workers, 10s measured after 2s warmup, loopback.

| Config | Req/s | p50 us | p99 us | p99.9 us | Errors | GCs | GC pause ms | Peak heap MB | GC CPU % | Peak RSS MB | GCs/10k req | CPU us/req |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| go | 122913 | 187 | 1192 | 2671 | 0 | 2903 | 680.01 | 3 | 4 | 19.5 | 19.87 | 27.60 |
| xo-arenas | 179282 | 142 | 921 | 2431 | 0 | 0 | 0.00 | 0 | 0 | 14.4 | 0.00 | 11.79 |
| node | 83685 | 344 | 920 | 1356 | 0 | 0 | 0.00 | 0 | 0 | 92.6 | 0.00 | 11.94 |
```
