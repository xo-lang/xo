# Native amd64 against LLVM on amd64 hosts, 2026-10-22

A re-measurement of bench/results/amd64-native-vs-llvm-2026-10-19.md on
main 5e06694d5 (Xo Lang 1, the lead's item, 2026-10-22): native amd64 is
the default backend on x86 hosts (decision 0270), so its gap to LLVM is
what users there get. Same programs and backends as 2026-10-19: LLVM at
the build defaults (-O2, ThinLTO), native amd64 with no flags.

Result: the gap has mostly closed. HTTP plain went from 1.24 to 1.05
times LLVM's instructions a request, the TLS handshake (tls-newconn) from
1.24 to 1.01, and tls and h2 are now below LLVM (0.85, 0.81). On the
compute benches (geometric mean of the Intel Mac's instructions, as
measured on 2026-10-19): JSON 1.24 to 1.06, RSA 1.44 to 1.01, portable
1.53 to 1.16, text 1.22 to 1.04 (callgrind on linux/amd64: 1.15, 1.05,
1.18, 1.11). Every checksum agrees
between the backends and between Docker and the Intel Mac.

## How

- Instructions: callgrind (valgrind 3.24.0) in Docker Desktop's emulated
  linux/amd64 (`scripts/docker-linux.sh amd64`), whose counts do not
  depend on load. HTTP: `ARCH=amd64 bench/perf/http/callgrind-docker.sh
  <darwin xo> OUT` (MODE=full: N=100, REPS=2, the minimum of the two
  repeats). Compute benches: each bench built in the container by a
  linux/amd64 xo (the Go backend cross build callgrind-docker.sh makes)
  with `--backend=amd64` and `--backend=llvm` (XO_INLINE_STRICT=1), each
  workload of the bench's measure.sh list run once under
  `valgrind --tool=callgrind`, the "Collected" total.
- Wall time: darwin/amd64 builds made on the arm64 Mac (`xo build
  --backend=amd64|llvm --target=darwin/amd64`), run on the Intel Mac
  (i7-9750H) 5 times each as `/usr/bin/time -l BIN WORKLOAD`: the minimum
  instructions retired and the minimum of the program's own
  microseconds (rsa and ctx print none). Load average 0.54 at the start,
  1.00 at the end.

## HTTP server, instructions a request (linux/amd64, callgrind)

| scenario | native amd64 | LLVM | native/LLVM | 2026-10-19 |
|---|---:|---:|---:|---:|
| plain | 29,398 | 27,959 | 1.051 | 1.242 |
| plain-newconn | 41,662 | 39,739 | 1.048 | 1.246 |
| tls | 39,986 | 47,346 | 0.845 | 1.023 |
| tls-newconn | 3,126,260 | 3,091,387 | 1.011 | 1.241 |
| h2 | 68,164 | 84,519 | 0.806 | 1.033 |

## Compute benches

Geometric means over each bench's workloads (without `none`, the setup
alone). "cg" is the callgrind count (linux/amd64), "Intel instr" the
Intel Mac's instructions retired, "wall" its microseconds; the
2026-10-19 column is that day's Intel instruction geomean.

| bench | workloads | cg geomean | min | max | Intel instr geomean | wall geomean | 2026-10-19 Intel instr |
|---|---:|---:|---:|---:|---:|---:|---:|
| atomic | 8 | 1.214 | 0.808 | 2.160 | 0.994 | 1.169 | 1.179 |
| ctx | 5 | 0.770 | 0.571 | 0.881 | 0.629 | - | 1.049 |
| digest | 5 | 0.925 | 0.735 | 1.031 | 0.956 | 0.955 | 1.027 |
| json | 11 | 1.145 | 0.968 | 1.360 | 1.063 | 1.147 | 1.237 |
| portable | 10 | 1.178 | 1.021 | 1.538 | 1.158 | 1.256 | 1.531 |
| rsa | 4 | 1.050 | 1.037 | 1.062 | 1.012 | - | 1.439 |
| text | 22 | 1.107 | 0.701 | 1.301 | 1.041 | 1.094 | 1.223 |

atomic's callgrind ratios (set 2.16, cas 1.71, get 1.55) are not the
program's: callgrind runs the bench's threads one at a time, and the
Intel Mac counts the same instructions for both backends there (set
1.001, cas 1.145, get 0.986). atomic get's wall ratio (2.76) is 1.4 ms
against 0.5 ms, too short a run to read.

Every workload:

| bench | workload | cg native | cg LLVM | cg ratio | Intel instr ratio | native us | LLVM us | wall ratio | checksums |
|---|---|---:|---:|---:|---:|---:|---:|---:|---|
| atomic | none | 48,579 | 52,236 | 0.930 | 0.700 | 0 | 0 | - | agree |
| atomic | get | 14,048,604 | 9,052,617 | 1.552 | 0.986 | 1399 | 507 | 2.759 | agree |
| atomic | set | 10,048,844 | 4,652,422 | 2.160 | 1.001 | 8595 | 8396 | 1.024 | agree |
| atomic | swap | 36,048,969 | 32,052,907 | 1.125 | 0.972 | 12132 | 11951 | 1.015 | agree |
| atomic | cas | 24,048,905 | 14,052,426 | 1.711 | 1.145 | 11660 | 11659 | 1.000 | agree |
| atomic | add | 68,048,951 | 78,052,817 | 0.872 | 0.858 | 16700 | 15031 | 1.111 | agree |
| atomic | max | 54,054,097 | 52,055,907 | 1.038 | 1.009 | 6181 | 5798 | 1.066 | agree |
| atomic | flag | 35,382,293 | 35,386,156 | 1.000 | 0.900 | 16128 | 16391 | 0.984 | agree |
| atomic | contended | 42,063,778 | 52,068,504 | 0.808 | 1.115 | 108945 | 104084 | 1.047 | agree |
| ctx | none | 54,549 | 59,636 | 0.915 | 0.701 | - | - | - | agree |
| ctx | defer | 524,048,005 | 674,051,793 | 0.777 | 0.686 | - | - | - | agree |
| ctx | mutex | 844,048,737 | 1,062,052,501 | 0.795 | 0.517 | - | - | - | agree |
| ctx | spawn | 30,686,289 | 35,120,593 | 0.874 | 0.728 | - | - | - | agree |
| ctx | budget | 664,074,540 | 1,163,746,058 | 0.571 | 0.516 | - | - | - | agree |
| ctx | await | 38,669,471 | 43,910,729 | 0.881 | 0.736 | - | - | - | agree |
| digest | none | 1,740,116 | 3,835,469 | 0.454 | 0.666 | 1 | 2 | 0.500 | agree |
| digest | sha256_big | 740,814,584 | 735,647,237 | 1.007 | 1.023 | 48211 | 49566 | 0.973 | agree |
| digest | sha256_small | 673,739,329 | 715,529,353 | 0.942 | 0.922 | 52359 | 57492 | 0.911 | agree |
| digest | hmac | 945,289,687 | 1,005,682,371 | 0.940 | 0.923 | 66318 | 76724 | 0.864 | agree |
| digest | pbkdf2 | 1,659,297,228 | 1,609,205,090 | 1.031 | 1.035 | 103484 | 113667 | 0.910 | agree |
| digest | argon2 | 216,527,527 | 294,532,182 | 0.735 | 0.884 | 21333 | 18769 | 1.137 | agree |
| json | none | 24,831,131 | 22,066,941 | 1.125 | 0.930 | 96 | 164 | 0.585 | agree |
| json | small | 377,421,855 | 300,298,976 | 1.257 | 1.168 | 30258 | 24979 | 1.211 | agree |
| json | ints | 233,439,929 | 205,730,808 | 1.135 | 1.064 | 18016 | 16866 | 1.068 | agree |
| json | floats | 395,855,349 | 381,558,132 | 1.037 | 1.009 | 39528 | 32120 | 1.231 | agree |
| json | strings | 377,663,090 | 325,836,722 | 1.159 | 1.081 | 29876 | 24951 | 1.197 | agree |
| json | maps | 357,790,286 | 315,316,162 | 1.135 | 1.085 | 29103 | 24870 | 1.170 | agree |
| json | parse | 318,528,930 | 234,212,407 | 1.360 | 1.330 | 24466 | 18035 | 1.357 | agree |
| json | invalid | 220,279,738 | 181,542,505 | 1.213 | 1.132 | 17126 | 13603 | 1.259 | agree |
| json | enc_small | 135,700,173 | 129,907,316 | 1.045 | 0.942 | 9979 | 9760 | 1.022 | agree |
| json | enc_strings | 213,944,934 | 220,908,102 | 0.968 | 0.890 | 15033 | 15016 | 1.001 | agree |
| json | enc_floats | 146,085,826 | 123,662,428 | 1.181 | 1.093 | 13900 | 11368 | 1.223 | agree |
| json | bytes | 474,248,424 | 410,847,380 | 1.154 | 0.971 | 37299 | 39179 | 0.952 | agree |
| portable | none | 1,692,715 | 1,464,259 | 1.156 | 0.783 | 7 | 13 | 0.538 | agree |
| portable | arith32 | 91,742,892 | 83,512,524 | 1.099 | 1.180 | 19038 | 16581 | 1.148 | agree |
| portable | arith8 | 75,740,778 | 69,500,806 | 1.090 | 1.023 | 8091 | 8018 | 1.009 | agree |
| portable | u64 | 73,789,861 | 53,550,486 | 1.378 | 1.227 | 9843 | 9323 | 1.056 | agree |
| portable | divmod | 297,588,939 | 245,514,473 | 1.212 | 1.382 | 66486 | 48113 | 1.382 | agree |
| portable | shift | 109,748,934 | 95,520,500 | 1.149 | 1.089 | 7841 | 7577 | 1.035 | agree |
| portable | conv | 214,290,929 | 139,332,494 | 1.538 | 1.510 | 17574 | 8570 | 2.051 | agree |
| portable | str_hash | 496,544,945 | 486,160,517 | 1.021 | 1.018 | 54394 | 43416 | 1.253 | agree |
| portable | unix | 108,442,983 | 98,120,518 | 1.105 | 1.062 | 8442 | 7073 | 1.194 | agree |
| portable | checked | 189,735,073 | 151,504,208 | 1.252 | 1.246 | 20842 | 19686 | 1.059 | agree |
| portable | float32 | 97,755,065 | 94,574,569 | 1.034 | 0.957 | 26522 | 15381 | 1.724 | agree |
| rsa | sign2048 | 1,046,991,572 | 985,441,677 | 1.062 | 1.055 | - | - | - | agree |
| rsa | verify2048 | 38,523,104 | 37,133,671 | 1.037 | 0.942 | - | - | - | agree |
| rsa | sign4096 | 6,291,831,517 | 5,949,144,828 | 1.058 | 1.054 | - | - | - | agree |
| rsa | verify4096 | 134,055,751 | 128,695,718 | 1.042 | 1.002 | - | - | - | agree |
| text | none | 7,489,674 | 6,803,505 | 1.101 | 0.850 | 65 | 110 | 0.591 | agree |
| text | parse_int | 36,063,206 | 32,878,438 | 1.097 | 0.978 | 3121 | 2760 | 1.131 | agree |
| text | dur_fmt | 150,032,258 | 130,193,625 | 1.152 | 1.079 | 15907 | 12678 | 1.255 | agree |
| text | dur_parse | 167,757,461 | 155,455,826 | 1.079 | 1.128 | 14311 | 11733 | 1.220 | agree |
| text | time_fmt | 196,358,646 | 155,626,609 | 1.262 | 1.197 | 17421 | 16986 | 1.026 | agree |
| text | time_parse | 133,758,698 | 106,496,724 | 1.256 | 1.168 | 10237 | 7926 | 1.292 | agree |
| text | quote | 139,615,889 | 127,766,669 | 1.093 | 1.007 | 10401 | 9933 | 1.047 | agree |
| text | split | 89,527,178 | 81,211,200 | 1.102 | 1.057 | 6357 | 5800 | 1.096 | agree |
| text | lines | 157,932,569 | 125,572,878 | 1.258 | 1.245 | 11767 | 9848 | 1.195 | agree |
| text | replace | 73,831,141 | 73,294,103 | 1.007 | 0.942 | 5246 | 5203 | 1.008 | agree |
| text | trim | 125,482,548 | 96,424,283 | 1.301 | 1.264 | 8107 | 6328 | 1.281 | agree |
| text | rune_count | 27,872,706 | 39,773,433 | 0.701 | 0.716 | 1745 | 2873 | 0.607 | agree |
| text | utf8 | 153,891,698 | 137,878,386 | 1.116 | 1.075 | 11598 | 10226 | 1.134 | agree |
| text | int_fmt | 70,485,898 | 63,596,955 | 1.108 | 0.969 | 5349 | 5684 | 0.941 | agree |
| text | float_fmt | 143,533,877 | 112,550,949 | 1.275 | 1.167 | 13480 | 11565 | 1.166 | agree |
| text | float_parse | 151,053,889 | 161,528,507 | 0.935 | 0.972 | 19978 | 14684 | 1.361 | agree |
| text | base64 | 182,037,289 | 147,946,449 | 1.230 | 1.025 | 13527 | 11346 | 1.192 | agree |
| text | hex | 112,148,370 | 100,336,329 | 1.118 | 1.006 | 7522 | 7286 | 1.032 | agree |
| text | debug | 258,500,071 | 254,316,241 | 1.016 | 0.933 | 22502 | 22768 | 0.988 | agree |
| text | zone_offset | 95,746,415 | 82,052,657 | 1.167 | 1.097 | 10107 | 8774 | 1.152 | agree |
| text | zone_fmt | 543,891,003 | 447,830,740 | 1.215 | 1.107 | 49506 | 45544 | 1.087 | agree |
| text | zone_civil | 275,900,002 | 251,303,787 | 1.098 | 1.013 | 25523 | 24054 | 1.061 | agree |
| text | zone_load | 36,357,807 | 37,664,934 | 0.965 | 0.919 | 3232 | 3012 | 1.073 | agree |

## Gaps, ranked (by what a user's program sees)

1. JSON parsing: json parse 1.36 times LLVM's instructions (callgrind)
   and 1.36 its time; small 1.26 and 1.21, invalid 1.21 and 1.26. The
   most used of these paths in a backend service; taken next (Xo Lang 1).
2. Narrow integer arithmetic LLVM proves in range: portable conv 1.54
   instructions and 2.05 time. In the loop, LLVM tests v >= 0 once and
   then computes every `%` by a constant as an unsigned 32 bit multiply
   and shift; native keeps the signed 64 bit sequence for each, tests the
   sign around each again, and runs a Rune's full validity test (three
   setcc, an or chain, and a redundant `xorq $1` pair). divmod 1.21 and
   1.38, u64 1.38 and 1.06.
3. Text formatting and scanning: trim 1.30 and 1.28, lines 1.26 and 1.20,
   float_fmt 1.28 and 1.17, time_fmt 1.26, time_parse 1.26 and 1.29.
4. portable float32: even instructions (0.96) but 1.72 the time: a stall
   in the loop (its carried Float32 dependency), not instruction count.

## Closing the JSON parse gap (2026-10-22 night, Xo Lang 1)

Per function (callgrind, linux/amd64, json parse): native spent 139M in
jsontext.scan, 108M in str_end (560,000 calls) and 41M in scalar_end
(320,000 calls); LLVM 207M in scan with both inlined. Two causes:

1. str_end's word loop made its five 64 bit masks and the Bytes data
   address again every 8 bytes on both native backends: a loop with two
   back edges (a `continue` in `while true`) had no preheader for
   hoist_consts, and x86 made a 64 bit logical immediate with a movabsq
   at each use (c94dc33c4, gotchas/loop-two-back-edges-no-hoist.md).
2. str_end (103 blocks) and scalar_end (133) were over the inliner's
   80 block limit for few call sites; each has one, so the copy is the
   only body. A one-site function is now inlined up to 500 instructions
   and 160 blocks (compiler/arm64 inline.xo INLINE_ONE_SITE_BLOCKS).

Instructions (callgrind, linux/amd64, bench/perf/json), main, step 1,
step 2; LLVM from the table above:

| workload | main | 1: hoisting | 2: one site | LLVM | native/LLVM |
|---|---:|---:|---:|---:|---:|
| parse | 318,529,935 | 312,444,975 | 255,550,524 | 234,212,407 | 1.091 |
| invalid | 220,279,738 | 220,630,463 | 192,224,488 | 181,542,505 | 1.059 |
| small | 377,421,855 | 377,309,015 | 348,947,219 | 300,298,976 | 1.162 |
| maps | 357,791,291 | 356,966,421 | 339,100,775 | 315,316,162 | 1.075 |
| ints | 233,439,929 | 232,094,289 | 219,420,798 | 205,730,808 | 1.067 |
| strings | 377,663,090 | 368,699,667 | 353,601,321 | 325,836,722 | 1.085 |
| bytes | 474,249,429 | 426,865,324 | 421,065,423 | 410,847,380 | 1.025 |

The encoders are unchanged. arm64 (Mac, `xo bench --ab --rounds=10`,
step 2 against step 1): instructions parse 0.913, small 0.971, strings
0.980, ints 0.971; wall within noise (runs of about 18 ms). Step 2 on
xo itself: __text 56,148,060 to 56,139,132 bytes, its build 86 s to 85
s; HTTP short callgrind (linux/amd64) plain 29,306 to 29,351, tls 38,667
to 37,007 instructions a request.
