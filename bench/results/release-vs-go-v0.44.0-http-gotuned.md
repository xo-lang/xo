# Xo 0.44.0 against Go at its defaults and with a memory limit: HTTP server

Measured 2026-11-09 by Xo Lang 4 on the linux/arm64 offload host 1 (12 CPUs,
go1.26.1), with scripts/release-vs-go-http.sh from 8513b2598 (its scripts and
bench/perf/jitter are the same as 26bda60dd's on main, which added the
go-tuned server). Xo is the released xo 0.44.0 (base is the same xo, a
same-binary control). Go is bench/perf/jitter/go at its defaults (`go`,
GOGC=100) and the same server with `GOGC=off GOMEMLIMIT=1GiB` (`go-tuned`).

Why: in release-vs-go-v0.38.1.md and later reports Go's /alloc row looked
weak (about 28,000 requests a second, p99 about 5 ms, over 100 us of CPU a
request, while Go's /hello is fast). Go's collector runs very often on this
server's small live heap; the GC counts at the end show it, and with a memory
limit in place of GOGC Go serves about 4.8 times its default rate. Quote both
Go columns.

## The run

- host load (start of run) 1.69 2.71 1.48 1/311 1037378
- release-vs-go-http l4rvg60690: 8513b2598 on Linux aarch64, 12 CPUs, go1.26.1
- xo xo 0.44.0 linux/arm64; base xo 0.44.0 linux/arm64
- go-tuned env: GOGC=off GOMEMLIMIT=1GiB
- loadgen flags: -c C -rate R -d 10s -warmup 2s -awake (default on in open loop); servers on CPUs 0-5, loadgen on 6-11
- Go's /alloc rows are its /alloc-hand (json.Marshal written by hand, as Xo's derive encodes).
- go is Go at its defaults (GOGC=100); go-tuned is the same server with the env above (a memory limit in place of GOGC): quote both.
- Rounds: 3 per row; each cell is the median, its range in brackets; CPU is the server's CPU microseconds per request.

| row | server | req/s | p50 us | p99 us | p99.9 us | max us | CPU us/req | peak RSS KB | errors |
|---|---|---|---|---|---|---|---|---|---|
| /alloc c32 | xo | 169,349 [169,324 to 170,436] | 172 [172 to 173] | 458 [452 to 461] | 1,259 [1,218 to 1,269] | 6,423 [6,197 to 6,630] | 33.9 [33.6 to 34.0] | 5,664 [4,540 to 5,704] | 0 [0 to 0] |
| /alloc c32 | base | 167,786 [167,676 to 168,967] | 175 [174 to 175] | 457 [451 to 460] | 1,242 [1,228 to 1,255] | 6,273 [6,143 to 6,308] | 34.1 [34.1 to 34.3] | 4,404 [4,336 to 5,668] | 0 [0 to 0] |
| /alloc c32 | go | 29,481 [28,181 to 30,024] | 425 [415 to 481] | 5,086 [4,904 to 5,492] | 7,441 [7,185 to 8,128] | 13,493 [13,219 to 13,508] | 105.2 [102.2 to 117.9] | 19,736 [18,356 to 19,784] | 0 [0 to 0] |
| /alloc c32 | go-tuned | 140,526 [140,243 to 141,625] | 204 [203 to 204] | 600 [596 to 602] | 1,211 [1,189 to 1,213] | 4,501 [4,157 to 10,545] | 41.3 [41.1 to 41.5] | 1,013,192 [1,013,124 to 1,014,812] | 0 [0 to 0] |
| /alloc c64 open 10000/s | xo | 10,000 [10,000 to 10,000] | 78 [78 to 79] | 110 [110 to 113] | 137 [134 to 148] | 226 [212 to 417] | 52.0 [51.4 to 52.0] | 6,016 [5,280 to 6,416] | 0 [0 to 0] |
| /alloc c64 open 10000/s | base | 10,000 [10,000 to 10,000] | 78 [78 to 79] | 114 [110 to 116] | 141 [139 to 143] | 206 [202 to 548] | 52.4 [51.8 to 52.6] | 5,116 [5,088 to 7,176] | 0 [0 to 0] |
| /alloc c64 open 10000/s | go | 10,000 [10,000 to 10,000] | 101 [100 to 102] | 386 [385 to 392] | 487 [487 to 513] | 932 [798 to 997] | 121.9 [121.3 to 123.3] | 17,496 [17,432 to 17,580] | 0 [0 to 0] |
| /alloc c64 open 10000/s | go-tuned | 10,000 [10,000 to 10,000] | 88 [88 to 88] | 128 [127 to 129] | 153 [146 to 154] | 522 [495 to 2,597] | 77.3 [77.0 to 77.8] | 1,013,476 [1,011,684 to 1,013,696] | 0 [0 to 0] |
| /alloc c64 open 20000/s | xo | 20,000 [20,000 to 20,000] | 91 [91 to 91] | 143 [141 to 144] | 166 [165 to 169] | 459 [216 to 2,188] | 57.5 [57.5 to 57.6] | 5,196 [5,060 to 5,864] | 0 [0 to 0] |
| /alloc c64 open 20000/s | base | 20,000 [20,000 to 20,000] | 92 [91 to 92] | 144 [144 to 145] | 168 [166 to 173] | 489 [226 to 1,615] | 57.9 [57.6 to 58.2] | 5,252 [5,012 to 5,652] | 0 [0 to 0] |
| /alloc c64 open 20000/s | go | 20,000 [20,000 to 20,000] | 187 [147 to 187] | 651 [563 to 660] | 888 [882 to 891] | 2,391 [1,884 to 8,986] | 127.3 [114.4 to 127.4] | 17,656 [17,388 to 17,968] | 0 [0 to 0] |
| /alloc c64 open 20000/s | go-tuned | 20,000 [20,000 to 20,000] | 89 [89 to 89] | 152 [151 to 152] | 215 [189 to 228] | 754 [716 to 955] | 61.0 [60.8 to 61.0] | 1,012,124 [1,012,036 to 1,012,280] | 0 [0 to 0] |
| /alloc c256 | xo | 177,945 [177,796 to 180,579] | 1,249 [1,233 to 1,256] | 5,154 [5,139 to 5,245] | 6,495 [6,407 to 6,583] | 10,167 [9,439 to 10,244] | 33.7 [33.2 to 33.7] | 10,956 [10,772 to 11,876] | 0 [0 to 0] |
| /alloc c256 | base | 179,309 [178,539 to 179,561] | 1,236 [1,235 to 1,244] | 5,218 [5,217 to 5,222] | 6,596 [6,480 to 6,683] | 10,553 [10,519 to 10,729] | 33.4 [33.4 to 33.6] | 11,056 [10,900 to 11,076] | 0 [0 to 0] |
| /alloc c256 | go | 57,479 [56,769 to 57,905] | 381 [379 to 401] | 38,204 [38,080 to 38,278] | 57,096 [57,075 to 57,493] | 118,313 [116,988 to 118,675] | 81.3 [80.7 to 82.2] | 41,044 [40,324 to 48,620] | 0 [0 to 0] |
| /alloc c256 | go-tuned | 140,810 [140,643 to 141,164] | 1,463 [1,459 to 1,464] | 5,944 [5,940 to 5,950] | 8,100 [7,851 to 8,182] | 14,628 [13,297 to 15,549] | 41.5 [41.3 to 41.5] | 1,014,880 [1,013,464 to 1,015,184] | 0 [0 to 0] |
| /hello c32 | xo | 359,094 [358,180 to 359,719] | 86 [85 to 86] | 219 [218 to 219] | 315 [314 to 317] | 4,200 [3,854 to 10,094] | 7.1 [7.1 to 7.2] | 4,660 [3,476 to 4,708] | 0 [0 to 0] |
| /hello c32 | base | 356,844 [170,398 to 360,645] | 86 [85 to 94] | 220 [219 to 2,029] | 327 [325 to 3,963] | 3,924 [3,613 to 12,372] | 7.1 [6.7 to 7.1] | 3,388 [3,300 to 3,516] | 0 [0 to 0] |
| /hello c32 | go | 278,936 [146,928 to 279,701] | 101 [101 to 105] | 627 [626 to 2,084] | 838 [836 to 5,514] | 3,874 [3,545 to 15,716] | 12.2 [12.1 to 12.2] | 17,760 [17,580 to 17,804] | 0 [0 to 0] |
| /hello c32 | go-tuned | 313,314 [311,803 to 315,375] | 98 [97 to 98] | 262 [262 to 264] | 365 [365 to 374] | 6,514 [3,850 to 10,102] | 11.3 [11.3 to 11.3] | 986,100 [984,032 to 986,116] | 0 [0 to 0] |
| /hello c256 | xo | 894,177 [891,896 to 900,666] | 256 [254 to 257] | 815 [810 to 816] | 1,683 [1,638 to 1,762] | 9,208 [8,464 to 10,167] | 5.8 [5.8 to 5.8] | 6,620 [6,484 to 8,116] | 0 [0 to 0] |
| /hello c256 | base | 897,504 [882,862 to 898,198] | 255 [255 to 258] | 816 [809 to 839] | 1,728 [1,694 to 1,819] | 7,619 [6,670 to 7,643] | 5.8 [5.8 to 5.9] | 6,700 [6,604 to 7,012] | 0 [0 to 0] |
| /hello c256 | go | 489,516 [488,369 to 495,756] | 308 [303 to 310] | 2,580 [2,575 to 2,610] | 4,235 [3,776 to 4,344] | 8,431 [7,994 to 8,767] | 10.0 [9.8 to 10.0] | 24,116 [22,332 to 24,332] | 0 [0 to 0] |
| /hello c256 | go-tuned | 732,005 [730,879 to 732,725] | 301 [300 to 301] | 1,083 [1,071 to 1,087] | 3,091 [2,851 to 3,122] | 9,079 [7,766 to 10,170] | 7.9 [7.8 to 7.9] | 986,028 [984,032 to 986,156] | 0 [0 to 0] |

- /alloc c32, xo/go medians: req/s 5.744, p50 us 0.405, p99 us 0.090, p99.9 us 0.169, max us 0.476, CPU us/req 0.323, peak RSS KB 0.287
- /alloc c32, xo/go-tuned medians: req/s 1.205, p50 us 0.843, p99 us 0.763, p99.9 us 1.040, max us 1.427, CPU us/req 0.821, peak RSS KB 0.006
- /alloc c32, xo/base medians: req/s 1.009, p50 us 0.983, p99 us 1.002, p99.9 us 1.014, max us 1.024, CPU us/req 0.995, peak RSS KB 1.286
- /alloc c64 open 10000/s, xo/go medians: req/s 1.000, p50 us 0.772, p99 us 0.285, p99.9 us 0.281, max us 0.242, CPU us/req 0.427, peak RSS KB 0.344
- /alloc c64 open 10000/s, xo/go-tuned medians: req/s 1.000, p50 us 0.886, p99 us 0.859, p99.9 us 0.895, max us 0.433, CPU us/req 0.673, peak RSS KB 0.006
- /alloc c64 open 10000/s, xo/base medians: req/s 1.000, p50 us 1.000, p99 us 0.965, p99.9 us 0.972, max us 1.097, CPU us/req 0.992, peak RSS KB 1.176
- /alloc c64 open 20000/s, xo/go medians: req/s 1.000, p50 us 0.487, p99 us 0.220, p99.9 us 0.187, max us 0.192, CPU us/req 0.452, peak RSS KB 0.294
- /alloc c64 open 20000/s, xo/go-tuned medians: req/s 1.000, p50 us 1.022, p99 us 0.941, p99.9 us 0.772, max us 0.609, CPU us/req 0.944, peak RSS KB 0.005
- /alloc c64 open 20000/s, xo/base medians: req/s 1.000, p50 us 0.989, p99 us 0.993, p99.9 us 0.988, max us 0.939, CPU us/req 0.994, peak RSS KB 0.989
- /alloc c256, xo/go medians: req/s 3.096, p50 us 3.278, p99 us 0.135, p99.9 us 0.114, max us 0.086, CPU us/req 0.414, peak RSS KB 0.267
- /alloc c256, xo/go-tuned medians: req/s 1.264, p50 us 0.854, p99 us 0.867, p99.9 us 0.802, max us 0.695, CPU us/req 0.812, peak RSS KB 0.011
- /alloc c256, xo/base medians: req/s 0.992, p50 us 1.011, p99 us 0.988, p99.9 us 0.985, max us 0.963, CPU us/req 1.007, peak RSS KB 0.991
- /hello c32, xo/go medians: req/s 1.287, p50 us 0.851, p99 us 0.349, p99.9 us 0.376, max us 1.084, CPU us/req 0.585, peak RSS KB 0.262
- /hello c32, xo/go-tuned medians: req/s 1.146, p50 us 0.878, p99 us 0.836, p99.9 us 0.863, max us 0.645, CPU us/req 0.630, peak RSS KB 0.005
- /hello c32, xo/base medians: req/s 1.006, p50 us 1.000, p99 us 0.995, p99.9 us 0.963, max us 1.070, CPU us/req 1.004, peak RSS KB 1.375
- /hello c256, xo/go medians: req/s 1.827, p50 us 0.831, p99 us 0.316, p99.9 us 0.397, max us 1.092, CPU us/req 0.581, peak RSS KB 0.275
- /hello c256, xo/go-tuned medians: req/s 1.222, p50 us 0.850, p99 us 0.753, p99.9 us 0.544, max us 1.014, CPU us/req 0.738, peak RSS KB 0.007
- /hello c256, xo/base medians: req/s 0.996, p50 us 1.004, p99 us 0.999, p99.9 us 0.974, max us 1.209, CPU us/req 0.998, peak RSS KB 0.988

## Go's collections on /alloc-hand c32

Go's server alone under each GC setting (Xo Lang 4's gobase-remote.sh: the
same server, CPU split and loadgen as above, `-c 32 -d 10s -warmup 2s`, 3
rounds, `GODEBUG=gctrace=1`). `gcs` counts the collections the server logged
over its whole run: the 2 s warm-up and the 10 s measured, about 12 s of load
(and a 0.5 s start). At its defaults Go collected 12,559 to 12,710 times, about
1,000 a second; with the 1 GiB limit, 115 times.

```
go version go1.26.1 linux/arm64 on aarch64 12 CPUs; load 0.07 0.68 0.43
hand-default r1 gcs=12559 rps 26883 p50 504 p99 5745 cpu_us 124.2
hand-default r2 gcs=12710 rps 27420 p50 496 p99 5525 cpu_us 121.0
hand-default r3 gcs=12626 rps 27136 p50 502 p99 5664 cpu_us 122.9
hand-gogc400 r1 gcs=5336 rps 94310 p50 204 p99 1613 cpu_us 50.8
hand-gogc400 r2 gcs=5422 rps 95792 p50 201 p99 1588 cpu_us 49.7
hand-gogc400 r3 gcs=5416 rps 95688 p50 201 p99 1588 cpu_us 49.9
hand-gcoff-limit1g r1 gcs=115 rps 140485 p50 204 p99 601 cpu_us 41.4
hand-gcoff-limit1g r2 gcs=115 rps 140431 p50 204 p99 601 cpu_us 41.4
hand-gcoff-limit1g r3 gcs=115 rps 140880 p50 204 p99 601 cpu_us 41.3
marshal-default r1 gcs=11704 rps 23179 p50 700 p99 7399 cpu_us 170.2
marshal-default r2 gcs=11575 rps 22852 p50 703 p99 7487 cpu_us 171.8
marshal-default r3 gcs=11727 rps 23015 p50 694 p99 7378 cpu_us 169.6
hello-default r1 gcs=2518 rps 280286 p50 100 p99 627 cpu_us 12.2
hello-default r2 gcs=2511 rps 279910 p50 101 p99 625 cpu_us 12.2
hello-default r3 gcs=2512 rps 280304 p50 101 p99 623 cpu_us 12.2

[exited with code 0]
```
