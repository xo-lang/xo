# Python against Xo: 2026-11-22 (HTTP rows)

Written by bench/perf/python/run.sh (twins/lib.sh). Ratios are Xo over Python (below 1: Xo is faster or smaller).

## Machine and versions

- Linux 7.0.14-linuxkit aarch64, host 1 (the linux/arm64 timing host, exclusive lease), 12 CPUs, load average at start 3.81
- xo 0.55.0 commit 9599fc7d key ff3b0011426efd26, NOT THIS CHECKOUT (its key is )
- go version go1.26.11 linux/arm64
- Python 3.13.5 (virtual environment of bench/perf/python from /usr/bin/python3.13)
- packages (requirements.txt, hashes): httptools==0.9.0 starlette==0.48.0 uvicorn==0.37.0 uvloop==0.23.0 


## HTTP: the users CRUD API (asyncio.Protocol and Starlette on uvicorn, one process each; Go net/http; Xo std/http)

```
# config rps p50_us p99_us p999_us errors gcs pause_ms peak_heap_mb gc_cpu_pct peak_rss_mb gcs_per_10k cpu_us_req
go 122721 187 1207 3107 0 2900 659.30 3 4 19.1 19.70 27.58
xo-arenas 180147 141 910 2363 0 0 0.00 0 0 14.0 0.00 11.91
python-asyncio 67099 441 1028 1345 0 0 0.00 0 0 19.0 0.00 14.83
python-starlette 39382 769 1622 1846 0 0 0.00 0 0 33.5 0.00 25.39
go 121224 187 1204 3392 0 2868 653.60 3 3 19.6 19.66 27.54
xo-arenas 177980 141 933 2647 0 0 0.00 0 0 13.8 0.00 11.83
python-asyncio 67763 437 1014 1295 0 0 0.00 0 0 18.7 0.00 14.70
python-starlette 39243 772 1627 1845 0 0 0.00 0 0 33.5 0.00 25.46
go 123737 186 1199 2658 0 2932 669.27 3 4 19.2 19.69 27.13
xo-arenas 179364 141 926 2463 0 0 0.00 0 0 14.3 0.00 11.93
python-asyncio 67771 437 1019 1317 0 0 0.00 0 0 18.7 0.00 14.68
python-starlette 39508 766 1617 1853 0 0 0.00 0 0 33.5 0.00 25.31

Medians of 3 runs, 32 workers, 10s measured after 2s warmup, loopback.

| Config | Req/s | p50 us | p99 us | p99.9 us | Errors | GCs | GC pause ms | Peak heap MB | GC CPU % | Peak RSS MB | GCs/10k req | CPU us/req |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| go | 122721 | 187 | 1204 | 3107 | 0 | 2900 | 659.30 | 3 | 4 | 19.2 | 19.69 | 27.54 |
| xo-arenas | 179364 | 141 | 926 | 2463 | 0 | 0 | 0.00 | 0 | 0 | 14.0 | 0.00 | 11.91 |
| python-asyncio | 67763 | 437 | 1019 | 1317 | 0 | 0 | 0.00 | 0 | 0 | 18.7 | 0.00 | 14.70 |
| python-starlette | 39382 | 769 | 1622 | 1846 | 0 | 0 | 0.00 | 0 | 0 | 33.5 | 0.00 | 25.39 |
```
