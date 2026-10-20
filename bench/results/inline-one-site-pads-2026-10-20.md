# Inlining functions with landing pads at their one call site (2026-10-20)

Decision 0256's addendum. HTTP callgrind (bench/perf/http/callgrind-docker.sh,
MODE=full: N=100, REPS=2, linux/arm64 in Docker on the Mac), the mean of
the two repeats, instructions a request; main 34585598.

| scenario      | main      | no cap    | cap 150   |
|---------------|-----------|-----------|-----------|
| plain         | 23,687    | 23,166 (-2.2%) | 23,234 (-1.9%) |
| plain-newconn | 31,256    | 30,726 (-1.7%) | 31,027 (-0.7%) |
| tls           | 36,849    | 36,261 (-1.6%) | 36,572 (-0.8%) |
| tls-newconn   | 1,882,423 | 1,881,908 (0.0%) | 1,881,105 (-0.1%) |
| h2            | 58,862    | 58,000 (-1.5%) | 58,720 (-0.2%) |

Raw lines: main plain 23679 23694, plain-newconn 31336 31176, tls 36814
36883, tls-newconn 1882165 1882680, h2 59488 58235; no cap plain 23151
23180, plain-newconn 30673 30779, tls 36258 36263, tls-newconn 1881728
1882087, h2 57877 58123; cap 150 plain 23263 23204, plain-newconn 30933
31121, tls 36494 36649, tls-newconn 1881503 1880707, h2 58638 58801.

MODE=short is too noisy for this: main alone gave plain 23324 and 23216,
tls 35494 and 36429 in two runs.

Text (`size -m`, __text), darwin/arm64:

| binary            | main       | no cap            | cap 150          | cap 300 |
|-------------------|------------|-------------------|------------------|---------|
| http server, native | 1,316,564 | 1,377,556 (+4.6%) | 1,330,084 (+1.0%) | 1,373,188 (+4.3%) |
| http server, LLVM (XO_LLVM_SPLIT=12) | 1,873,704 | 1,873,704 | (same) | |
| xo, native        | 33,992,316 | 34,744,964 (+2.2%) | | |

The growth is in the copies, not left bodies (the server's text symbols
went from 1723 to 1600 without a cap): a copied function's pads chain
into the caller's cleanups.
