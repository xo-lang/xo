# Xo against Go: the HTTP section on linux/arm64 host 1 (2026-11-10)

scripts/release-vs-go.sh SECTIONS=http, started from the Mac Studio; the section ran on linux/arm64
offload host 1 (Linux aarch64, 12 CPUs, 39 GB) with xo built from main 6fe10a957 (with 4d66f9752's
script) as "xo" and the 0.44.0 linux-arm64 release archive as "base", Go 1.26.1. The 0.46.0 candidate
(2517a0b66) is later main (44b668bdd). Its own lines below give the run's host load and flags.

## HTTP server (bench/perf/jitter, linux/arm64 offload host)

- host load 0.48 0.10 0.23 1/311 1891628
- release-vs-go-http rvg60401: 4d66f9752e on Linux aarch64, 12 CPUs, go1.26.1
- xo xo 0.45.0 linux/arm64; base xo 0.44.0 linux/arm64
- go-tuned env: GOGC=off GOMEMLIMIT=1GiB
- loadgen flags: -c C -rate R -d 10s -warmup 2s -awake (default on in open loop); servers on CPUs 0-5, loadgen on 6-11
- Go's /alloc rows are its /alloc-hand (json.Marshal written by hand, as Xo's derive encodes).
- go is Go at its defaults (GOGC=100); go-tuned is the same server with the env above (a memory limit in place of GOGC): quote both.
- Built at each XO_ARM64_PAD of HTTP_PADS: the table pools every pad's rounds; the pad sweep verdict below judges xo against base.
- Rounds: 12 per row; each cell is the median, its range in brackets; CPU is the server's CPU microseconds per request.

| row | server | req/s | p50 us | p99 us | p99.9 us | max us | CPU us/req | peak RSS KB | errors |
|---|---|---|---|---|---|---|---|---|---|
| /alloc c32 | xo | 170,708 [165,965 to 178,695] | 170 [164 to 174] | 478 [441 to 498] | 1,080 [1,046 to 1,244] | 6,219 [4,959 to 8,901] | 32.2 [31.4 to 33.2] | 4,548 [4,404 to 5,512] | 0 [0 to 0] |
| /alloc c32 | base | 160,828 [157,171 to 168,365] | 181 [174 to 185] | 498 [456 to 520] | 1,232 [1,160 to 1,289] | 6,266 [5,904 to 6,868] | 34.8 [34.0 to 35.5] | 4,478 [4,356 to 4,540] | 0 [0 to 0] |
| /alloc c32 | go | 28,270 [27,822 to 29,076] | 487 [450 to 490] | 5,410 [5,131 to 5,518] | 7,911 [7,551 to 8,129] | 13,988 [13,425 to 14,726] | 117.4 [110.1 to 118.6] | 19,768 [19,732 to 21,904] | 0 [0 to 0] |
| /alloc c32 | go-tuned | 134,609 [130,492 to 140,796] | 211 [204 to 218] | 659 [603 to 679] | 1,542 [1,247 to 1,640] | 4,471 [3,637 to 5,541] | 42.3 [41.3 to 43.7] | 1,013,020 [1,012,976 to 1,014,884] | 0 [0 to 0] |
| /alloc c64 open 10000/s | xo | 10,000 [10,000 to 10,000] | 75 [75 to 76] | 110 [107 to 123] | 136 [127 to 158] | 296 [198 to 1,239] | 49.5 [48.7 to 50.2] | 5,354 [4,960 to 7,036] | 0 [0 to 0] |
| /alloc c64 open 10000/s | base | 10,000 [10,000 to 10,000] | 78 [77 to 79] | 114 [108 to 122] | 136 [129 to 149] | 276 [195 to 1,311] | 52.2 [51.6 to 53.4] | 5,290 [4,876 to 6,512] | 0 [0 to 0] |
| /alloc c64 open 10000/s | go | 10,000 [10,000 to 10,000] | 100 [97 to 100] | 387 [355 to 388] | 498 [474 to 509] | 2,036 [725 to 2,447] | 121.8 [113.5 to 122.1] | 17,636 [17,472 to 17,684] | 0 [0 to 0] |
| /alloc c64 open 10000/s | go-tuned | 10,000 [10,000 to 10,000] | 88 [88 to 88] | 128 [127 to 132] | 154 [151 to 160] | 713 [519 to 844] | 77.3 [76.9 to 77.7] | 1,011,884 [1,011,824 to 1,011,972] | 0 [0 to 0] |
| /alloc c64 open 20000/s | xo | 20,000 [20,000 to 20,000] | 89 [88 to 89] | 139 [138 to 140] | 164 [162 to 170] | 304 [219 to 2,863] | 55.0 [54.2 to 55.5] | 4,948 [4,640 to 6,172] | 0 [0 to 0] |
| /alloc c64 open 20000/s | base | 20,000 [20,000 to 20,000] | 92 [91 to 92] | 144 [142 to 145] | 171 [167 to 175] | 368 [215 to 884] | 58.2 [57.5 to 59.1] | 5,098 [4,720 to 7,080] | 0 [0 to 0] |
| /alloc c64 open 20000/s | go | 20,000 [20,000 to 20,000] | 193 [190 to 196] | 731 [680 to 754] | 3,438 [980 to 3,589] | 10,205 [3,062 to 13,266] | 128.8 [128.0 to 129.7] | 17,672 [17,388 to 17,744] | 0 [0 to 0] |
| /alloc c64 open 20000/s | go-tuned | 20,000 [20,000 to 20,000] | 89 [89 to 89] | 152 [151 to 152] | 197 [195 to 222] | 714 [692 to 738] | 60.8 [60.8 to 61.1] | 1,014,084 [1,012,096 to 1,014,196] | 0 [0 to 0] |
| /alloc c256 | xo | 192,669 [188,154 to 197,388] | 1,166 [1,131 to 1,192] | 4,914 [4,878 to 4,983] | 6,262 [6,068 to 6,432] | 10,819 [9,392 to 14,517] | 30.9 [30.4 to 31.7] | 11,066 [10,812 to 12,048] | 0 [0 to 0] |
| /alloc c256 | base | 176,798 [173,442 to 180,978] | 1,270 [1,233 to 1,300] | 5,170 [5,065 to 5,221] | 6,635 [6,358 to 6,718] | 10,890 [10,071 to 13,241] | 33.8 [33.1 to 34.4] | 10,908 [10,664 to 12,536] | 0 [0 to 0] |
| /alloc c256 | go | 57,088 [57,070 to 62,154] | 390 [328 to 392] | 37,725 [34,747 to 38,530] | 56,159 [51,625 to 57,685] | 119,051 [105,401 to 127,128] | 81.8 [74.2 to 81.9] | 40,548 [38,264 to 44,756] | 0 [0 to 0] |
| /alloc c256 | go-tuned | 139,783 [138,851 to 141,599] | 1,484 [1,453 to 1,501] | 5,998 [5,890 to 6,022] | 8,039 [7,851 to 8,237] | 15,269 [14,515 to 15,398] | 41.8 [41.2 to 42.0] | 1,015,388 [1,013,548 to 1,016,256] | 0 [0 to 0] |
| /hello c32 | xo | 349,973 [348,564 to 352,587] | 87 [86 to 87] | 228 [226 to 230] | 408 [368 to 433] | 3,968 [3,725 to 5,863] | 7.5 [7.4 to 7.5] | 3,334 [3,100 to 3,500] | 0 [0 to 0] |
| /hello c32 | base | 352,247 [347,288 to 355,205] | 86 [85 to 87] | 227 [226 to 234] | 402 [379 to 441] | 4,138 [3,655 to 4,876] | 7.3 [7.3 to 7.4] | 3,144 [2,960 to 3,536] | 0 [0 to 0] |
| /hello c32 | go | 271,626 [270,829 to 272,231] | 103 [102 to 103] | 635 [635 to 638] | 868 [864 to 868] | 3,682 [3,449 to 4,082] | 12.6 [12.5 to 12.6] | 17,868 [17,752 to 17,924] | 0 [0 to 0] |
| /hello c32 | go-tuned | 304,420 [304,040 to 308,614] | 99 [98 to 99] | 277 [274 to 278] | 459 [458 to 481] | 4,179 [3,744 to 4,404] | 11.6 [11.4 to 11.6] | 985,984 [983,988 to 986,160] | 0 [0 to 0] |
| /hello c256 | xo | 867,203 [864,733 to 905,531] | 260 [253 to 262] | 892 [800 to 925] | 1,884 [1,732 to 2,137] | 7,321 [6,339 to 11,723] | 6.1 [5.9 to 6.1] | 6,768 [6,564 to 7,852] | 0 [0 to 0] |
| /hello c256 | base | 875,940 [857,571 to 905,864] | 258 [253 to 263] | 886 [803 to 928] | 2,006 [1,819 to 2,121] | 8,380 [6,548 to 10,137] | 6.0 [5.8 to 6.0] | 6,674 [6,480 to 7,908] | 0 [0 to 0] |
| /hello c256 | go | 476,093 [475,758 to 493,414] | 315 [302 to 316] | 2,694 [2,622 to 2,694] | 4,774 [4,587 to 4,780] | 9,639 [9,210 to 11,489] | 10.2 [9.8 to 10.2] | 23,980 [23,944 to 24,340] | 0 [0 to 0] |
| /hello c256 | go-tuned | 707,965 [705,574 to 736,043] | 305 [299 to 307] | 1,174 [1,066 to 1,180] | 3,178 [2,787 to 3,223] | 7,973 [7,088 to 8,092] | 8.0 [7.9 to 8.0] | 986,124 [986,052 to 988,212] | 0 [0 to 0] |

- /alloc c32, xo/go medians: req/s 6.039, p50 us 0.350, p99 us 0.088, p99.9 us 0.137, max us 0.445, CPU us/req 0.274, peak RSS KB 0.230
- /alloc c32, xo/go-tuned medians: req/s 1.268, p50 us 0.808, p99 us 0.725, p99.9 us 0.700, max us 1.391, CPU us/req 0.760, peak RSS KB 0.004
- /alloc c32, xo/base medians: req/s 1.061, p50 us 0.942, p99 us 0.959, p99.9 us 0.877, max us 0.992, CPU us/req 0.924, peak RSS KB 1.016
- /alloc c64 open 10000/s, xo/go medians: req/s 1.000, p50 us 0.750, p99 us 0.284, p99.9 us 0.273, max us 0.145, CPU us/req 0.406, peak RSS KB 0.304
- /alloc c64 open 10000/s, xo/go-tuned medians: req/s 1.000, p50 us 0.852, p99 us 0.859, p99.9 us 0.883, max us 0.415, CPU us/req 0.640, peak RSS KB 0.005
- /alloc c64 open 10000/s, xo/base medians: req/s 1.000, p50 us 0.962, p99 us 0.969, p99.9 us 1.004, max us 1.072, CPU us/req 0.947, peak RSS KB 1.012
- /alloc c64 open 20000/s, xo/go medians: req/s 1.000, p50 us 0.461, p99 us 0.190, p99.9 us 0.048, max us 0.030, CPU us/req 0.427, peak RSS KB 0.280
- /alloc c64 open 20000/s, xo/go-tuned medians: req/s 1.000, p50 us 1.000, p99 us 0.914, p99.9 us 0.832, max us 0.426, CPU us/req 0.905, peak RSS KB 0.005
- /alloc c64 open 20000/s, xo/base medians: req/s 1.000, p50 us 0.973, p99 us 0.962, p99.9 us 0.959, max us 0.826, CPU us/req 0.945, peak RSS KB 0.971
- /alloc c256, xo/go medians: req/s 3.375, p50 us 2.990, p99 us 0.130, p99.9 us 0.111, max us 0.091, CPU us/req 0.378, peak RSS KB 0.273
- /alloc c256, xo/go-tuned medians: req/s 1.378, p50 us 0.786, p99 us 0.819, p99.9 us 0.779, max us 0.709, CPU us/req 0.740, peak RSS KB 0.011
- /alloc c256, xo/base medians: req/s 1.090, p50 us 0.918, p99 us 0.951, p99.9 us 0.944, max us 0.993, CPU us/req 0.914, peak RSS KB 1.014
- /hello c32, xo/go medians: req/s 1.288, p50 us 0.845, p99 us 0.359, p99.9 us 0.469, max us 1.078, CPU us/req 0.594, peak RSS KB 0.187
- /hello c32, xo/go-tuned medians: req/s 1.150, p50 us 0.879, p99 us 0.823, p99.9 us 0.888, max us 0.950, CPU us/req 0.641, peak RSS KB 0.003
- /hello c32, xo/base medians: req/s 0.994, p50 us 1.012, p99 us 1.004, p99.9 us 1.015, max us 0.959, CPU us/req 1.018, peak RSS KB 1.060
- /hello c256, xo/go medians: req/s 1.821, p50 us 0.825, p99 us 0.331, p99.9 us 0.395, max us 0.760, CPU us/req 0.596, peak RSS KB 0.282
- /hello c256, xo/go-tuned medians: req/s 1.225, p50 us 0.852, p99 us 0.759, p99.9 us 0.593, max us 0.918, CPU us/req 0.759, peak RSS KB 0.007
- /hello c256, xo/base medians: req/s 0.990, p50 us 1.006, p99 us 1.006, p99.9 us 0.939, max us 0.874, CPU us/req 1.013, peak RSS KB 1.014

pad sweep verdict (reference base): each cell is the median over pads of each pad's median over rounds, the pad medians' range in brackets; slower or faster only when the ranges do not overlap

| row | group | pads | req/s | CPU us/req | req/s vs ref | verdict |
|---|---|---|---|---|---|---|
| /alloc c32 | base | 4 | 159,323 [158,394 to 162,376] | 35.13 [34.30 to 35.35] |  |  |
| /alloc c32 | go | 1 | 28,270 [28,270 to 28,270] | 117.37 [117.37 to 117.37] | 0.177 | one pad: not judged |
| /alloc c32 | go-tuned | 1 | 134,609 [134,609 to 134,609] | 42.31 [42.31 to 42.31] | 0.845 | one pad: not judged |
| /alloc c32 | xo | 4 | 169,503 [167,403 to 171,428] | 32.43 [31.92 to 33.04] | 1.064 | faster |
| /alloc c64 open 10000/s | base | 4 | 10,000 [10,000 to 10,000] | 52.55 [51.60 to 52.90] |  |  |
| /alloc c64 open 10000/s | go | 1 | 10,000 [10,000 to 10,000] | 121.80 [121.80 to 121.80] | 1.000 | one pad: not judged |
| /alloc c64 open 10000/s | go-tuned | 1 | 10,000 [10,000 to 10,000] | 77.30 [77.30 to 77.30] | 1.000 | one pad: not judged |
| /alloc c64 open 10000/s | xo | 4 | 10,000 [10,000 to 10,000] | 49.65 [49.30 to 49.90] | 1.000 | slower |
| /alloc c64 open 20000/s | base | 4 | 20,000 [20,000 to 20,000] | 58.27 [57.70 to 58.65] |  |  |
| /alloc c64 open 20000/s | go | 1 | 20,000 [20,000 to 20,000] | 128.80 [128.80 to 128.80] | 1.000 | one pad: not judged |
| /alloc c64 open 20000/s | go-tuned | 1 | 20,000 [20,000 to 20,000] | 60.80 [60.80 to 60.80] | 1.000 | one pad: not judged |
| /alloc c64 open 20000/s | xo | 4 | 20,000 [20,000 to 20,000] | 55.17 [54.60 to 55.35] | 1.000 | placement |
| /alloc c256 | base | 4 | 175,822 [173,769 to 178,689] | 33.93 [33.34 to 34.31] |  |  |
| /alloc c256 | go | 1 | 57,088 [57,088 to 57,088] | 81.79 [81.79 to 81.79] | 0.325 | one pad: not judged |
| /alloc c256 | go-tuned | 1 | 139,783 [139,783 to 139,783] | 41.79 [41.79 to 41.79] | 0.795 | one pad: not judged |
| /alloc c256 | xo | 4 | 192,116 [188,942 to 193,153] | 31.02 [30.86 to 31.53] | 1.093 | faster |
| /hello c32 | base | 4 | 352,006 [351,287 to 353,740] | 7.33 [7.28 to 7.34] |  |  |
| /hello c32 | go | 1 | 271,626 [271,626 to 271,626] | 12.57 [12.57 to 12.57] | 0.772 | one pad: not judged |
| /hello c32 | go-tuned | 1 | 304,420 [304,420 to 304,420] | 11.65 [11.65 to 11.65] | 0.865 | one pad: not judged |
| /hello c32 | xo | 4 | 349,973 [349,443 to 350,939] | 7.48 [7.43 to 7.49] | 0.994 | slower |
| /hello c256 | base | 4 | 875,727 [872,779 to 879,186] | 5.97 [5.96 to 6.01] |  |  |
| /hello c256 | go | 1 | 476,093 [476,093 to 476,093] | 10.16 [10.16 to 10.16] | 0.544 | one pad: not judged |
| /hello c256 | go-tuned | 1 | 707,965 [707,965 to 707,965] | 7.98 [7.98 to 7.98] | 0.808 | one pad: not judged |
| /hello c256 | xo | 4 | 867,203 [865,325 to 870,102] | 6.06 [6.03 to 6.07] | 0.990 | slower |
