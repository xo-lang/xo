# Xo 0.55.0 against Go: HTTP server, linux/arm64 (2026-11-21)

scripts/release-vs-go.sh's http section on host 1 (task #645): three leases of 30 minutes or less (x4h1 to x4h3), each one round at XO_ARM64_PAD 0, 1, 2 and 3, merged here (12 rounds a row). Xo is the v0.55.0 linux-arm64 trial archive; base is xo 0.54.0.

- host load 0.49 2.17 4.97 1/289 668181
- release-vs-go-http x4h1: 9599fc7d078 on Linux aarch64, 12 CPUs, go1.26.11
- xo xo 0.55.0 linux/arm64; base xo 0.54.0 linux/arm64
- go-tuned env: GOGC=off GOMEMLIMIT=1GiB
- loadgen flags: -c C -rate R -d 10s -warmup 2s -awake (default on in open loop); servers on CPUs 0-5, loadgen on 6-11
- host load 0.65 3.62 5.24 1/288 673961
- release-vs-go-http x4h2: 9599fc7d078 on Linux aarch64, 12 CPUs, go1.26.11
- xo xo 0.55.0 linux/arm64; base xo 0.54.0 linux/arm64
- go-tuned env: GOGC=off GOMEMLIMIT=1GiB
- loadgen flags: -c C -rate R -d 10s -warmup 2s -awake (default on in open loop); servers on CPUs 0-5, loadgen on 6-11
- host load 3.53 5.14 5.91 1/288 677742
- release-vs-go-http x4h3: 9599fc7d078 on Linux aarch64, 12 CPUs, go1.26.11
- xo xo 0.55.0 linux/arm64; base xo 0.54.0 linux/arm64
- go-tuned env: GOGC=off GOMEMLIMIT=1GiB
- loadgen flags: -c C -rate R -d 10s -warmup 2s -awake (default on in open loop); servers on CPUs 0-5, loadgen on 6-11
- Go's /alloc rows are its /alloc-hand (json.Marshal written by hand, as Xo's derive encodes).
- go is Go at its defaults (GOGC=100); go-tuned is the same server with the env above (a memory limit in place of GOGC): quote both.
- Built at each XO_ARM64_PAD of HTTP_PADS: the table pools every pad's rounds; the pad sweep verdict below judges xo against base.
- Rounds: 12 per row; each cell is the median, its range in brackets; CPU is the server's CPU microseconds per request.

| row | server | req/s | p50 us | p99 us | p99.9 us | max us | CPU us/req | peak RSS KB | errors |
|---|---|---|---|---|---|---|---|---|---|
| /alloc c32 | xo | 172,589 [167,091 to 176,443] | 172 [167 to 177] | 438 [431 to 451] | 828 [751 to 969] | 4,470 [4,238 to 6,374] | 33.2 [32.4 to 34.4] | 4,388 [4,236 to 6,444] | 0 [0 to 0] |
| /alloc c32 | base | 171,490 [164,756 to 173,139] | 172 [171 to 180] | 442 [437 to 456] | 846 [773 to 915] | 4,610 [4,203 to 9,302] | 33.5 [33.0 to 34.9] | 4,310 [4,252 to 6,408] | 0 [0 to 0] |
| /alloc c32 | go | 28,262 [28,238 to 28,292] | 486 [486 to 486] | 5,452 [5,415 to 5,500] | 8,089 [7,976 to 8,116] | 13,814 [13,602 to 15,614] | 118.3 [118.1 to 118.5] | 19,948 [19,792 to 20,064] | 0 [0 to 0] |
| /alloc c32 | go-tuned | 132,220 [130,316 to 133,003] | 214 [214 to 218] | 679 [663 to 682] | 1,642 [1,604 to 1,644] | 4,504 [4,399 to 7,665] | 42.9 [42.9 to 43.8] | 1,012,888 [1,011,424 to 1,013,192] | 0 [0 to 0] |
| /alloc c64 open 10000/s | xo | 10,000 [10,000 to 10,000] | 76 [75 to 78] | 116 [111 to 121] | 148 [131 to 414] | 366 [244 to 5,940] | 51.0 [49.6 to 52.0] | 5,588 [5,120 to 6,656] | 0 [0 to 0] |
| /alloc c64 open 10000/s | base | 10,000 [10,000 to 10,000] | 76 [75 to 80] | 116 [109 to 124] | 147 [127 to 301] | 827 [199 to 3,589] | 50.9 [50.1 to 53.7] | 5,294 [5,056 to 8,636] | 0 [0 to 0] |
| /alloc c64 open 10000/s | go | 10,000 [9,989 to 10,000] | 99 [99 to 100] | 389 [388 to 393] | 524 [497 to 543] | 5,634 [811 to 10,660] | 121.2 [120.6 to 121.8] | 17,204 [15,840 to 17,484] | 0 [0 to 0] |
| /alloc c64 open 10000/s | go-tuned | 10,000 [10,000 to 10,000] | 87 [86 to 87] | 128 [127 to 131] | 161 [160 to 161] | 694 [462 to 1,211] | 76.3 [75.9 to 76.7] | 1,012,052 [1,011,932 to 1,013,876] | 0 [0 to 0] |
| /alloc c64 open 20000/s | xo | 20,000 [20,000 to 20,000] | 89 [88 to 92] | 140 [138 to 149] | 170 [163 to 198] | 307 [246 to 4,350] | 56.1 [54.9 to 57.8] | 5,308 [4,748 to 6,464] | 0 [0 to 0] |
| /alloc c64 open 20000/s | base | 20,000 [20,000 to 20,000] | 90 [88 to 92] | 141 [139 to 153] | 172 [166 to 227] | 512 [272 to 2,555] | 56.6 [55.5 to 58.0] | 5,248 [4,848 to 7,468] | 0 [0 to 0] |
| /alloc c64 open 20000/s | go | 20,000 [20,000 to 20,000] | 194 [193 to 195] | 779 [762 to 840] | 3,930 [3,548 to 7,580] | 14,995 [13,536 to 20,490] | 128.8 [128.1 to 129.3] | 17,740 [17,588 to 19,684] | 0 [0 to 0] |
| /alloc c64 open 20000/s | go-tuned | 20,000 [20,000 to 20,000] | 88 [88 to 89] | 157 [154 to 160] | 497 [330 to 517] | 4,350 [4,050 to 5,526] | 60.7 [60.6 to 60.9] | 1,014,496 [1,012,340 to 1,014,536] | 0 [0 to 0] |
| /alloc c256 | xo | 186,712 [182,060 to 192,176] | 1,313 [1,277 to 1,347] | 3,282 [3,168 to 3,396] | 4,280 [4,148 to 4,809] | 7,431 [6,790 to 17,703] | 32.0 [31.1 to 32.8] | 10,716 [10,472 to 12,456] | 0 [0 to 0] |
| /alloc c256 | base | 183,333 [178,053 to 187,426] | 1,334 [1,309 to 1,378] | 3,373 [3,255 to 3,460] | 4,514 [4,108 to 4,782] | 7,752 [7,122 to 16,339] | 32.5 [31.8 to 33.5] | 10,708 [10,464 to 11,876] | 0 [0 to 0] |
| /alloc c256 | go | 57,217 [56,932 to 57,220] | 400 [395 to 402] | 37,685 [37,381 to 37,732] | 56,109 [55,580 to 56,738] | 103,875 [97,736 to 104,637] | 81.9 [81.9 to 82.3] | 40,308 [40,248 to 44,160] | 0 [0 to 0] |
| /alloc c256 | go-tuned | 139,411 [138,272 to 139,600] | 1,499 [1,495 to 1,501] | 5,962 [5,960 to 6,081] | 7,857 [7,745 to 8,613] | 15,712 [13,049 to 16,109] | 41.9 [41.8 to 42.2] | 1,015,520 [1,012,772 to 1,015,760] | 0 [0 to 0] |
| /hello c32 | xo | 364,054 [360,644 to 365,130] | 81 [80 to 82] | 227 [225 to 228] | 448 [399 to 494] | 4,226 [3,564 to 9,125] | 7.7 [7.7 to 7.9] | 3,182 [2,852 to 4,272] | 0 [0 to 0] |
| /hello c32 | base | 360,919 [358,256 to 364,359] | 82 [81 to 82] | 228 [226 to 233] | 491 [415 to 592] | 4,180 [3,523 to 6,761] | 7.7 [7.7 to 7.8] | 3,218 [3,020 to 5,820] | 0 [0 to 0] |
| /hello c32 | go | 273,557 [271,594 to 274,210] | 102 [101 to 102] | 637 [635 to 639] | 876 [870 to 896] | 4,423 [4,225 to 8,734] | 12.5 [12.5 to 12.6] | 17,780 [17,736 to 17,888] | 0 [0 to 0] |
| /hello c32 | go-tuned | 306,382 [304,548 to 306,788] | 99 [98 to 99] | 276 [275 to 276] | 467 [453 to 495] | 4,296 [4,248 to 4,368] | 11.6 [11.6 to 11.6] | 986,248 [984,240 to 986,256] | 0 [0 to 0] |
| /hello c256 | xo | 883,072 [862,396 to 892,592] | 254 [252 to 258] | 873 [858 to 919] | 2,443 [2,251 to 2,667] | 9,696 [8,076 to 11,444] | 6.5 [6.4 to 6.6] | 6,742 [6,444 to 10,408] | 0 [0 to 0] |
| /hello c256 | base | 889,468 [790,190 to 894,670] | 253 [251 to 272] | 868 [852 to 1,113] | 2,466 [2,048 to 2,666] | 8,139 [7,287 to 9,495] | 6.5 [6.4 to 7.0] | 6,706 [6,504 to 9,292] | 0 [0 to 0] |
| /hello c256 | go | 479,421 [478,383 to 481,072] | 315 [313 to 316] | 2,664 [2,648 to 2,675] | 4,379 [4,340 to 4,405] | 15,382 [9,772 to 15,927] | 10.2 [10.1 to 10.2] | 24,044 [23,864 to 24,480] | 0 [0 to 0] |
| /hello c256 | go-tuned | 710,194 [692,739 to 711,816] | 305 [304 to 309] | 1,162 [1,161 to 1,244] | 3,137 [3,109 to 3,267] | 9,056 [8,327 to 9,519] | 8.0 [8.0 to 8.2] | 986,196 [986,060 to 988,168] | 0 [0 to 0] |

- /alloc c32, xo/go medians: req/s 6.107, p50 us 0.353, p99 us 0.080, p99.9 us 0.102, max us 0.324, CPU us/req 0.280, peak RSS KB 0.220
- /alloc c32, xo/go-tuned medians: req/s 1.305, p50 us 0.801, p99 us 0.644, p99.9 us 0.504, max us 0.992, CPU us/req 0.773, peak RSS KB 0.004
- /alloc c32, xo/base medians: req/s 1.006, p50 us 0.994, p99 us 0.990, p99.9 us 0.978, max us 0.970, CPU us/req 0.991, peak RSS KB 1.018
- /alloc c64 open 10000/s, xo/go medians: req/s 1.000, p50 us 0.768, p99 us 0.298, p99.9 us 0.281, max us 0.065, CPU us/req 0.420, peak RSS KB 0.325
- /alloc c64 open 10000/s, xo/go-tuned medians: req/s 1.000, p50 us 0.874, p99 us 0.906, p99.9 us 0.916, max us 0.528, CPU us/req 0.668, peak RSS KB 0.006
- /alloc c64 open 10000/s, xo/base medians: req/s 1.000, p50 us 1.000, p99 us 1.000, p99.9 us 1.003, max us 0.443, CPU us/req 1.000, peak RSS KB 1.056
- /alloc c64 open 20000/s, xo/go medians: req/s 1.000, p50 us 0.459, p99 us 0.180, p99.9 us 0.043, max us 0.020, CPU us/req 0.436, peak RSS KB 0.299
- /alloc c64 open 20000/s, xo/go-tuned medians: req/s 1.000, p50 us 1.011, p99 us 0.895, p99.9 us 0.343, max us 0.071, CPU us/req 0.923, peak RSS KB 0.005
- /alloc c64 open 20000/s, xo/base medians: req/s 1.000, p50 us 0.994, p99 us 0.996, p99.9 us 0.991, max us 0.600, CPU us/req 0.991, peak RSS KB 1.011
- /alloc c256, xo/go medians: req/s 3.263, p50 us 3.283, p99 us 0.087, p99.9 us 0.076, max us 0.072, CPU us/req 0.390, peak RSS KB 0.266
- /alloc c256, xo/go-tuned medians: req/s 1.339, p50 us 0.876, p99 us 0.550, p99.9 us 0.545, max us 0.473, CPU us/req 0.763, peak RSS KB 0.011
- /alloc c256, xo/base medians: req/s 1.018, p50 us 0.984, p99 us 0.973, p99.9 us 0.948, max us 0.959, CPU us/req 0.984, peak RSS KB 1.001
- /hello c32, xo/go medians: req/s 1.331, p50 us 0.794, p99 us 0.356, p99.9 us 0.512, max us 0.955, CPU us/req 0.620, peak RSS KB 0.179
- /hello c32, xo/go-tuned medians: req/s 1.188, p50 us 0.818, p99 us 0.822, p99.9 us 0.960, max us 0.984, CPU us/req 0.667, peak RSS KB 0.003
- /hello c32, xo/base medians: req/s 1.009, p50 us 0.988, p99 us 0.993, p99.9 us 0.913, max us 1.011, CPU us/req 1.001, peak RSS KB 0.989
- /hello c256, xo/go medians: req/s 1.842, p50 us 0.808, p99 us 0.328, p99.9 us 0.558, max us 0.630, CPU us/req 0.636, peak RSS KB 0.280
- /hello c256, xo/go-tuned medians: req/s 1.243, p50 us 0.834, p99 us 0.751, p99.9 us 0.779, max us 1.071, CPU us/req 0.810, peak RSS KB 0.007
- /hello c256, xo/base medians: req/s 0.993, p50 us 1.006, p99 us 1.005, p99.9 us 0.991, max us 1.191, CPU us/req 1.005, peak RSS KB 1.005
