# Python against Xo: 2026-10-23

Written by bench/perf/python/run.sh (twins/lib.sh). Ratios are Xo over Python (below 1: Xo is faster or smaller).

## Machine and versions

- Darwin 27.0.0 arm64, Apple M4 Pro, 12 CPUs, load average at start 22.26
- xo 0.26.0 commit d5e048e2 key 61b0f62b356c5063, dirty fff69bed4bf0
- go version go1.26.1 darwin/arm64
- Python 3.13.12 (Homebrew python3.13, virtual environment of bench/perf/python)
- packages (requirements.txt, hashes): httptools==0.9.0 starlette==0.48.0 uvicorn==0.37.0 uvloop==0.23.0 

## Compute (recursive fib 40, 3 rounds)

| fib | load 4.80 |
    wall_ms               9532        235.4       1.3% 23.67%       0.025   0.024 to 0.037    too few rounds (n=3)
    cpu_ms                9500        230.0       1.421% 6.522%     0.024   0.023 to 0.027    too few rounds (n=3)
    instructions          1685171466163327728937  0.003% 0.137%     0.02    0.02 to 0.02      too few rounds (n=3)
    max_rss_kb            14064       2608        0.171% 0.92%      0.185   0.184 to 0.188    too few rounds (n=3)

## Concurrency (asyncio, threading, multiprocessing; no rwmutex row: the standard library has no read write lock)

| spawn | load 4.78 |
    wall_ms               452.1       60.89       1.292% 10.73%     0.135   0.123 to 0.143    better (n=10)
    cpu_ms                430.0       220.0       0% 15.91%         0.512   0.465 to 0.558    better (n=10)
    instructions          8229869907  519004676   0.416% 6.086%     0.063   0.061 to 0.066    better (n=10)
    max_rss_kb            25368       7000        0.489% 5.486%     0.276   0.271 to 0.289    better (n=10)
| fanout | load 4.67 |
    wall_ms               171.7       28.41       1.309% 18.75%     0.165   0.16 to 0.191     better (n=10)
    cpu_ms                150.0       95.0        5.0% 18.42%       0.633   0.563 to 0.733    better (n=10)
    instructions          3088525306  280344173   0.101% 3.909%     0.091   0.09 to 0.094     better (n=10)
    max_rss_kb            25448       7456        0.157% 4.721%     0.293   0.282 to 0.299    better (n=10)
| pingpong | load 4.41 |
    wall_ms               2900        26.19       4.236% 6.111%     0.009   0.009 to 0.009    better (n=10)
    cpu_ms                780.0       20.0        2.244% 0%         0.026   0.025 to 0.026    better (n=10)
    instructions          9229620714  296131492   0.199% 0.094%     0.032   0.032 to 0.032    better (n=10)
    max_rss_kb            23816       2712        0.739% 1.622%     0.114   0.112 to 0.115    better (n=10)
| mutex | load 4.17 |
    wall_ms               71.85       10.95       3.373% 2.728%     0.152   0.15 to 0.158     better (n=10)
    cpu_ms                50.0        10.0        15.0% 0%          0.2     0.167 to 0.2      too coarse (n=10)
    instructions          970390561   158075358   0.169% 1.331%     0.163   0.162 to 0.174    better (n=10)
    max_rss_kb            23856       3400        0.469% 4.353%     0.143   0.137 to 0.144    better (n=10)
| pmap | load 4.40 |
    wall_ms               3847        73.8        1.906% 3.549%     0.019   0.018 to 0.02     too few rounds (n=3)
    cpu_ms                31840       610.0       1.665% 0%         0.019   0.019 to 0.02     too few rounds (n=3)
    instructions          911679800   3738271373  0.514% 0.036%     4.1     4.085 to 4.128    too few rounds (n=3)
    max_rss_kb            26336       5088        0.243% 0.786%     0.193   0.19 to 0.194     too few rounds (n=3)

## JSON (json.loads and json.dumps against std/json; 1000 users, 100 rounds)

| json decode, Python | load 4.72 |
    wall_ms               142.7       35.88       1.832% 2.058%     0.251   0.247 to 0.256    better (n=10)
    cpu_ms                130.0       30.0        0% 0%             0.231   0.231 to 0.231    better (n=10)
    instructions          2914204794  626681821   0.717% 0.02%      0.215   0.214 to 0.216    better (n=10)
    max_rss_kb            15408       3200        0.286% 0.5%       0.208   0.207 to 0.209    better (n=10)
| json decode, Go | load 4.72 |
    wall_ms               99.48       35.58       2.494% 1.596%     0.358   0.351 to 0.362    better (n=10)
    cpu_ms                90.0        30.0        0% 0%             0.333   0.333 to 0.333    too coarse (n=10)
    instructions          2126794529  626639066   0.079% 0.012%     0.295   0.294 to 0.295    better (n=10)
    max_rss_kb            11176       3200        3.686% 0%         0.286   0.277 to 0.291    better (n=10)
| json encode, Python | load 4.90 |
    wall_ms               146.8       18.84       2.42% 2.228%      0.128   0.125 to 0.13     better (n=10)
    cpu_ms                130.0       10.0        0% 0%             0.077   0.074 to 0.077    better (n=10)
    instructions          2915656128  292156865   0.285% 0.073%     0.1     0.1 to 0.1        better (n=10)
    max_rss_kb            15160       2992        0.449% 0%         0.197   0.196 to 0.198    better (n=10)
| json encode, Go | load 4.90 |
    wall_ms               27.58       19.94       3.733% 3.944%     0.723   0.699 to 0.759    better (n=10)
    cpu_ms                20.0        10.0        0% 0%             0.5     0.5 to 0.5        too coarse (n=10)
    instructions          557400341   292174951   0.184% 0.029%     0.524   0.524 to 0.525    better (n=10)
    max_rss_kb            10568       3016        1.136% 1.326%     0.285   0.283 to 0.289    better (n=10)

## HTTP: the users CRUD API (asyncio.Protocol and Starlette on uvicorn, one process each; Go net/http; Xo std/http)

```
# config rps p50_us p99_us p999_us errors gcs pause_ms peak_heap_mb gc_cpu_pct peak_rss_mb gcs_per_10k cpu_us_req
go 96849 263 1425 3638 0 2868 424.95 5 4 23.5 24.48 31.98
xo-arenas 124491 218 972 2336 0 0 0.00 0 0 18.1 0.00 18.50
python-asyncio 80776 363 894 1213 0 0 0.00 0 0 23.5 0.00 12.22
python-starlette 32842 930 1958 2264 0 0 0.00 0 0 37.6 0.00 30.30
go 105988 243 1254 3102 0 3103 437.79 4 4 23.1 24.21 31.71
xo-arenas 134468 201 937 2316 0 0 0.00 0 0 19.1 0.00 18.03
python-asyncio 83423 346 909 1618 0 0 0.00 0 0 23.0 0.00 11.71
python-starlette 32463 922 2114 4028 0 0 0.00 0 0 37.5 0.00 30.37
go 104697 241 1310 3592 0 3036 419.63 4 4 23.1 23.91 31.09
xo-arenas 137989 198 888 2053 0 0 0.00 0 0 19.2 0.00 17.94
python-asyncio 82798 349 906 2049 0 0 0.00 0 0 23.1 0.00 11.80
python-starlette 32782 925 1970 4318 0 0 0.00 0 0 37.7 0.00 30.23

Medians of 3 runs, 32 workers, 10s measured after 2s warmup, loopback.

| Config | Req/s | p50 us | p99 us | p99.9 us | Errors | GCs | GC pause ms | Peak heap MB | GC CPU % | Peak RSS MB | GCs/10k req | CPU us/req |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| go | 104697 | 243 | 1310 | 3592 | 0 | 3036 | 424.95 | 4 | 4 | 23.1 | 24.21 | 31.71 |
| xo-arenas | 134468 | 201 | 937 | 2316 | 0 | 0 | 0.00 | 0 | 0 | 19.1 | 0.00 | 18.03 |
| python-asyncio | 82798 | 349 | 906 | 1618 | 0 | 0 | 0.00 | 0 | 0 | 23.1 | 0.00 | 11.80 |
| python-starlette | 32782 | 925 | 1970 | 4028 | 0 | 0 | 0.00 | 0 | 0 | 37.6 | 0.00 | 30.30 |
```

## Notes

- Not a quiet machine: other agents' jobs ran during the measurements. Every row waited for the 1 minute load average to drop under 5 and reports the load it started at; the slow rows (fib, pmap) ran 3 rounds ("too few rounds" in the verdict) because one Python run is seconds long; treat the ratios as orders of magnitude, not decimals.
- Every program prints a checksum and the runner stops when Python's and Xo's differ; both CRUD servers were checked against the Go server on 26 requests by bench/perf/http/parity.sh.
- Python is CPython 3.13 from Homebrew in its own virtual environment (bench/perf/python/.venv, packages pinned with hashes in requirements.txt), with the GIL: mutex runs 8 threads under threading.Lock, pmap uses multiprocessing (one process per CPU), spawn, fanout and pingpong are asyncio tasks. There is no rwmutex row: the standard library has no read write lock. The CRUD server is given twice: the standard library's asyncio.Protocol, and Starlette on uvicorn (httptools, uvloop), one process each.
- Wall, instructions and RSS include the interpreter start (about 15 ms).
