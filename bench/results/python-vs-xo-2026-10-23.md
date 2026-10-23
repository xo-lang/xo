# Python against Xo: 2026-10-23

Written by bench/perf/python/run.sh (twins/lib.sh). Ratios are Xo over Python (below 1: Xo is faster or smaller).

## Machine and versions

- Darwin 27.0.0 arm64, Apple M4 Pro, 12 CPUs, load average at start 22.67
- xo 0.26.0 commit 71c71cf9 key ef38c10e5b5c60dd
- go version go1.26.1 darwin/arm64
- Python 3.13.12 (Homebrew python3.13, virtual environment of bench/perf/python)
- packages (requirements.txt, hashes): httptools==0.9.0 starlette==0.48.0 uvicorn==0.37.0 uvloop==0.23.0 

## Compute (recursive fib 40, 3 rounds)

| fib | load 4.75 |
    wall_ms               9025        221.8       1.031% 1.674%     0.025   0.024 to 0.025    too few rounds (n=3)
    cpu_ms                9010        210.0       0.943% 2.381%     0.023   0.023 to 0.024    too few rounds (n=3)
    instructions          1685266482093327348292  0.004% 0.012%     0.02    0.02 to 0.02      too few rounds (n=3)
    max_rss_kb            14032       2624        0.114% 0.915%     0.187   0.186 to 0.189    too few rounds (n=3)

## Concurrency (asyncio, threading, multiprocessing; no rwmutex row: the standard library has no read write lock)

| spawn | load 3.77 |
    wall_ms               436.2       69.45       2.196% 4.457%     0.159   0.152 to 0.163    better (n=10)
    cpu_ms                410.0       295.0       2.439% 11.86%     0.72    0.643 to 0.756    better (n=10)
    instructions          8225364795  644718503   0.222% 4.134%     0.078   0.075 to 0.08     better (n=10)
    max_rss_kb            25288       7480        0.443% 2.032%     0.296   0.289 to 0.299    better (n=10)
| fanout | load 4.19 |
    wall_ms               178.3       33.62       1.817% 4.122%     0.189   0.185 to 0.195    better (n=10)
    cpu_ms                160.0       120.0       6.25% 8.333%      0.75    0.667 to 0.758    better (n=10)
    instructions          3096365878  322050304   0.094% 2.921%     0.104   0.103 to 0.106    better (n=10)
    max_rss_kb            25656       7304        0.795% 6.024%     0.285   0.272 to 0.289    better (n=10)
| pingpong | load 4.50 |
    wall_ms               2857        26.44       1.692% 5.386%     0.009   0.009 to 0.015    better (n=10)
    cpu_ms                760.0       20.0        5.592% 0%         0.026   0.025 to 0.027    better (n=10)
    instructions          9237862912  296011643   0.133% 0.137%     0.032   0.032 to 0.032    better (n=10)
    max_rss_kb            23784       2728        0.841% 1.613%     0.115   0.113 to 0.116    better (n=10)
| mutex | load 3.62 |
    wall_ms               68.76       10.84       3.529% 1.487%     0.158   0.154 to 0.161    better (n=10)
    cpu_ms                50.0        10.0        0% 0%             0.2     0.2 to 0.2        too coarse (n=10)
    instructions          972444088   188790387   0.179% 1.786%     0.194   0.191 to 0.195    better (n=10)
    max_rss_kb            23800       3392        0.118% 2.712%     0.143   0.138 to 0.143    better (n=10)
| pmap | load 4.45 |
    wall_ms               3187        61.82       1.024% 4.313%     0.019   0.019 to 0.021    too few rounds (n=3)
    cpu_ms                29920       590.0       0.568% 0.847%     0.02    0.02 to 0.02      too few rounds (n=3)
    instructions          851162536   3742553747  1.183% 0.016%     4.397   4.327 to 4.431    too few rounds (n=3)
    max_rss_kb            26192       5424        0.153% 0.442%     0.207   0.205 to 0.208    too few rounds (n=3)

## JSON (json.loads and json.dumps against std/json; 1000 users, 100 rounds)

| json decode, Python | load 4.45 |
    wall_ms               135.1       34.46       1.147% 1.683%     0.255   0.252 to 0.259    better (n=10)
    cpu_ms                120.0       30.0        0% 0%             0.25    0.25 to 0.25      better (n=10)
    instructions          2920891735  627156451   0.674% 0.029%     0.215   0.214 to 0.216    better (n=10)
    max_rss_kb            15384       3216        0.208% 0.498%     0.209   0.208 to 0.21     better (n=10)
| json decode, Go | load 4.45 |
    wall_ms               96.78       34.27       0.651% 1.517%     0.354   0.353 to 0.359    better (n=10)
    cpu_ms                90.0        30.0        0% 0%             0.333   0.333 to 0.333    too coarse (n=10)
    instructions          2127287812  626991920   0.071% 0.007%     0.295   0.295 to 0.295    better (n=10)
    max_rss_kb            10896       3200        1.322% 1.125%     0.294   0.287 to 0.298    better (n=10)
| json encode, Python | load 4.66 |
    wall_ms               139.3       18.0        1.519% 2.971%     0.129   0.127 to 0.131    better (n=10)
    cpu_ms                130.0       10.0        5.769% 0%         0.077   0.077 to 0.083    better (n=10)
    instructions          2913213184  292235074   0.272% 0.019%     0.1     0.1 to 0.1        better (n=10)
    max_rss_kb            15160       2992        0.211% 0%         0.197   0.197 to 0.198    better (n=10)
| json encode, Go | load 4.66 |
    wall_ms               25.03       18.11       2.589% 3.645%     0.723   0.698 to 0.738    better (n=10)
    cpu_ms                20.0        10.0        0% 0%             0.5     0.5 to 0.5        too coarse (n=10)
    instructions          557210711   292222433   0.301% 0.008%     0.524   0.523 to 0.525    better (n=10)
    max_rss_kb            10328       2992        3.408% 0%         0.29    0.282 to 0.294    better (n=10)

## HTTP: the users CRUD API (asyncio.Protocol and Starlette on uvicorn, one process each; Go net/http; Xo std/http)

```
# config rps p50_us p99_us p999_us errors gcs pause_ms peak_heap_mb gc_cpu_pct peak_rss_mb gcs_per_10k cpu_us_req
go 122884 219 935 1700 0 3444 434.57 4 4 23.0 23.48 33.89
xo-arenas 146370 196 671 1100 0 0 0.00 0 0 20.0 0.00 23.15
python-asyncio 83250 354 843 1113 0 0 0.00 0 0 23.2 0.00 11.90
python-starlette 34156 898 1872 2066 0 0 0.00 0 0 37.6 0.00 29.16
go 115421 228 1057 2080 0 3808 486.57 4 5 22.9 27.48 34.27
xo-arenas 150398 192 651 1068 0 0 0.00 0 0 18.7 0.00 23.38
python-asyncio 83792 351 842 1249 0 0 0.00 0 0 23.0 0.00 11.79
python-starlette 33127 919 1938 2831 0 0 0.00 0 0 37.8 0.00 30.01
go 85052 280 1839 4685 0 2558 402.75 4 3 23.1 23.91 27.42
xo-arenas 138844 202 791 1595 0 0 0.00 0 0 17.6 0.00 18.72
python-asyncio 50683 424 2828 7752 0 0 0.00 0 0 23.1 0.00 15.57
python-starlette 24839 970 4635 9544 0 0 0.00 0 0 37.8 0.00 36.60

Medians of 3 runs, 32 workers, 10s measured after 2s warmup, loopback.

| Config | Req/s | p50 us | p99 us | p99.9 us | Errors | GCs | GC pause ms | Peak heap MB | GC CPU % | Peak RSS MB | GCs/10k req | CPU us/req |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| go | 115421 | 228 | 1057 | 2080 | 0 | 3444 | 434.57 | 4 | 4 | 23.0 | 23.91 | 33.89 |
| xo-arenas | 146370 | 196 | 671 | 1100 | 0 | 0 | 0.00 | 0 | 0 | 18.7 | 0.00 | 23.15 |
| python-asyncio | 83250 | 354 | 843 | 1249 | 0 | 0 | 0.00 | 0 | 0 | 23.1 | 0.00 | 11.90 |
| python-starlette | 33127 | 919 | 1938 | 2831 | 0 | 0 | 0.00 | 0 | 0 | 37.8 | 0.00 | 30.01 |
```
