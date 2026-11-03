# Xo against Go: xo 0.36.0 darwin/arm64 (2026-11-03)

The release comparison (toolchain.md "Cutting a release" step 7) on the
released darwin/arm64 archive of v0.36.0 (tag 5942fb280, signed, not yet
notarized: the notary profile was missing, gotchas/notary-profile-missing.md;
notarization does not change the binaries), scripts/release-vs-go.sh from
the tag's tree with SECTIONS="compile run test" as HEAVY_CLASS=measure on
the Mac (2026-11-03 02:10 to 02:19, after a first try waited two hours on
heavyd's priority self-wait, since fixed), 3 rounds per row (the script's
default since 2026-11-01; 0.35.0's report ran 10). The HTTP section ran on
the linux/arm64 offload host on 2026-11-02 and follows the script's report.
The comparison against the previous release's report
(release-vs-go-v0.35.0.md) is each run row's B/A ratio (Xo over Go), old
against new, a row worse by more than 3% flagged; flagged wall rows were
then measured directly, 0.35.0's build against 0.36.0's of the same
program on the linux/arm64 offload host (ab-host.sh, 10 rounds).

## Against 0.35.0

Wall ratios against Go: 22 of 24 rows faster than Go (the 20 run rows and
the 4 test rows); nbody slower (1.056; 0.35.0: 1.059) and pmap slower
(1.044; 0.35.0: 0.988, within its noise then).

Flagged wall rows, settled by a direct A/B of the two releases' builds
(linux/arm64, 10 rounds): llvm strings 0.460 to 0.532 (+15.7%): wall 1.021,
0.996 to 1.034, within noise. coll push 0.731 to 0.849 (+16.1%): Xo's own median moved (9.02 to 10.37
ms; Go's 12.34 to 12.21). On darwin/arm64 the direct A/B of coll built by
the two archives (10 rounds, load 4.1) confirms it: wall 1.152, 1.074 to
1.259, worse; cpu 1.133. On linux/arm64 the same A/B is 0.952, 0.89 to
0.974, better: a darwin only slowdown (cause below). Smaller flags (update +4.1%, copymut +5.4%, spawn +4.0%, pmap
+5.7%) are within the 3-round spreads; cpu flags are 10 ms granularity.

Memory, worse: Xo's coll rows peak about 1.2 MB higher on the Mac (putget
7,376 to 8,608 KB, update 7,352 to 8,608 KB; the llvm rows unchanged, fib
0.623 to 0.621 of Go). The direct A/Bs agree: darwin putget max_rss 1.182
(7,280 to 8,608 KB) with wall 1.025 (within noise), darwin push 1.101
(50,928 to 56,064 KB), linux push 1.048 (55,440 to 58,096 KB). On linux/arm64 the
direct A/B of putget has wall 1.006 (within noise) and max_rss 1.317
(10,352 to 13,634 KB, 1.246 to 1.388), worse. The push slowdown is in std,
not the compiler: 0.36.0's xo building coll against v0.35.0's std runs
push at 0.88 of the stock build (8.97 against 10.19 ms, darwin). Ruled
out on darwin: the large block cache advice (XO_HEAP_CACHE_ADVISE=0,
still 1.111) and the small first chunks (decision 0455; off, 0.987).
A bisect over the 145 std commits of v0.35.0..v0.36.0 (coll built by
0.36.0's xo against each commit's std, push wall against v0.35.0's std,
6 rounds a step, slow over 1.06) names 188398ede, decision 0452's list
growth curve (Go's slice curve: doubling up to 256 elements, then about
1.25 times): 1.102 there, 0.938 at its parent. 0452 traded it
for memory (the self build's max RSS 0.935 on linux/arm64, 20 million
pushes 301 to 212 MB on the Mac); coll push, which grows many lists past
256 elements, pays more reallocations on darwin and none of it shows on
linux/arm64 (0.952). The coll RSS rise is not settled by this bisect (it
ran on wall only).

HTTP against 0.35.0 (linux/arm64, 5 rounds a row): /hello within 1.2%
(c32 0.995, c256 0.988 req/s); /alloc 4 to 5% slower with 5 to 6% more CPU
per request (c32 0.960 req/s, CPU 1.055; c256 0.949, CPU 1.054; open loop
p50 +2.6 to +3.4%). Against Go: /alloc 5.9x (c32) and 3.1x (c256) Go's
req/s, /hello 1.29x and 1.84x.

Test rows: json cold ran loaded in all 3 rounds (load 8.4 to 10.4), so its
ratios are not reliable; the others ran unloaded.

Run rows, B/A (Xo over Go) old (0.35.0's report) against new:

| workload | wall old | wall new | cpu old | cpu new | rss old | rss new | flag |
|---|---|---|---|---|---|---|---|
| llvm fib | 0.965 | 0.968 | 1.000 | 0.966 | 0.623 | 0.621 |  |
| llvm sieve | 0.946 | 0.973 | 0.588 | 0.629 | 0.402 | 0.489 | cpu +7.0%, max +21.6% |
| llvm nbody | 1.059 | 1.056 | 1.067 | 1.055 | 0.614 | 0.616 |  |
| llvm sort | 0.805 | 0.821 | 0.778 | 0.771 | 0.450 | 0.624 | max +38.7% |
| llvm interp | 0.321 | 0.304 | 0.286 | 0.300 | 0.621 | 0.602 | cpu +4.9% |
| llvm collatz | 0.898 | 0.919 | 0.896 | 0.918 | 0.616 | 0.618 |  |
| llvm strings | 0.460 | 0.532 | 0.250 | 0.375 | 0.622 | 0.701 | wall +15.7%, cpu +50.0%, max +12.7% |
| coll putget | 0.595 | 0.600 | 0.500 | 0.589 | 0.840 | 0.982 | cpu +17.8%, max +16.9% |
| coll get | 0.618 | 0.623 | 0.500 | 0.605 | 0.976 | 1.011 | cpu +21.0%, max +3.6% |
| coll push | 0.731 | 0.849 | 0.000 | 0.588 | 0.728 | 1.032 | wall +16.1%, max +41.8% |
| coll update | 0.616 | 0.641 | 0.500 | 0.628 | 0.831 | 0.993 | wall +4.1%, cpu +25.6%, max +19.5% |
| coll iter | 0.274 | 0.280 | 0.000 | 0.253 | 0.980 | 1.016 | max +3.7% |
| coll copymut | 0.632 | 0.666 | 0.000 | 0.613 | 0.687 | 0.714 | wall +5.4%, max +3.9% |
| coll branch | 0.025 | 0.024 | 0.000 | 0.013 | 0.438 | 0.476 | max +8.7% |
| conc spawn | 0.426 | 0.443 | 0.133 | 0.192 | 0.492 | 0.574 | wall +4.0%, cpu +44.4%, max +16.7% |
| conc fanout | 0.687 | 0.686 | 0.200 | 0.240 | 0.405 | 0.384 | cpu +20.0% |
| conc pingpong | 0.873 | 0.877 | 1.000 | 0.625 | 0.652 | 0.664 |  |
| conc mutex | 0.279 | 0.271 | 0.000 | 0.056 | 0.567 | 0.566 |  |
| conc rwmutex | 0.901 | 0.892 | 1.000 | 0.508 | 0.554 | 0.579 | max +4.5% |
| conc pmap | 0.988 | 1.044 | 0.982 | 1.013 | 0.796 | 0.816 | wall +5.7%, cpu +3.2% |
| base64 (encoding/base64) cold | - | 0.566 | - | 0.236 | - | 1.145 |  |
| base64 (encoding/base64) edit | - | 0.815 | - | 0.403 | - | 1.859 |  |
| json (encoding/json) cold | - | 0.336 | - | 0.130 | - | 1.167 |  |
| json (encoding/json) edit | - | 0.236 | - | 0.120 | - | 1.310 |  |

wall faster than Go: 22 of 24

# Xo against Go: xo 0.36.0 darwin/arm64 (2026-11-03)

scripts/release-vs-go.sh on Darwin arm64, 24 GB,
Go go1.26.1. Build times are wall milliseconds of one build; peak MB is the
build's largest resident set (cold / warm / edit). Warm is the third unchanged build
(xo's parts pack is written on the second and read from the third); second is the second.
Reports through v0.35.0 ran xo's timed builds under XO_CACHE_STATS=1, which added about
28 MB to coll's warm peak (116 against 88 MB, 2026-11-01), so xo's peak MB steps down
from those reports to this one with no change in the compiler.
The edit's path is what xo's cache statistics say an untimed build of another edit did: separate (against
std's variants, built after the cold row as a user's second build would have them),
with the std records, or whole (the whole program).
xo's builds run with the whole-build run cache off (XO_BIN_CACHE=off), so the second and
warm rows are compiles through the back-end caches, not a cached result (before 2026-10-29
they could be the run cache's 10 to 20 ms).
Size is the default build's binary.

## Compile and size

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB | edit's path |
|---|---|---|---|---|---|---|---|---|
| llvm | go | 1410 | 30 | 40 | 100 | 295 / 26 / 68 | 2.39 | - |
| coll | go | 1370 | 30 | 30 | 100 | 310 / 25 / 69 | 2.43 | - |
| conc | go | 1370 | 30 | 30 | 90 | 287 / 26 / 66 | 2.40 | - |
| llvm-fib | xo | 840 | 40 | 30 | 30 | 248 / 38 / 38 | 1.27 | separate, std records |
| llvm-sieve | xo | 850 | 50 | 30 | 40 | 249 / 38 / 39 | 1.27 | separate, std records |
| llvm-nbody | xo | 890 | 50 | 40 | 40 | 248 / 39 / 40 | 1.29 | separate, std records |
| llvm-sort | xo | 880 | 60 | 40 | 40 | 248 / 38 / 39 | 1.27 | separate, std records |
| llvm-interp | xo | 970 | 60 | 40 | 40 | 249 / 40 / 40 | 1.29 | separate, std records |
| llvm-collatz | xo | 920 | 50 | 30 | 30 | 248 / 38 / 39 | 1.27 | separate, std records |
| llvm-strings | xo | 880 | 50 | 30 | 30 | 247 / 38 / 38 | 1.27 | separate, std records |
| coll | xo | 920 | 130 | 100 | 100 | 254 / 47 / 47 | 1.39 | separate, std records |
| conc-spawn | xo | 1040 | 50 | 30 | 30 | 247 / 39 / 38 | 1.37 | separate, std records |
| conc-fanout | xo | 1040 | 50 | 30 | 40 | 249 / 39 / 38 | 1.38 | separate, std records |
| conc-pingpong | xo | 1040 | 50 | 40 | 40 | 247 / 39 / 40 | 1.41 | separate, std records |
| conc-mutex | xo | 1040 | 50 | 40 | 30 | 249 / 41 / 40 | 1.42 | separate, std records |
| conc-rwmutex | xo | 1030 | 50 | 40 | 40 | 250 / 41 / 40 | 1.43 | separate, std records |
| conc-pmap | xo | 1040 | 50 | 40 | 40 | 249 / 41 / 40 | 1.38 | separate, std records |

The Go column builds one binary per group (llvm, coll, conc); the Xo
programs are one per workload except coll.

## Run (xo bench --ab, A = Go, B = Xo, 3 rounds)

### llvm fib (load 5 at start)

```
A: llvm.go fib 
B: llvm-fib.xo fib 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.13/5.13 5.13/5.13

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               227.2       219.8       5.44% 0.359%      0.968   0.875 to 0.979    better (n=3)
cpu_ms                226.6       219.0       5.48% 0.404%      0.966   0.874 to 0.979    better (n=3)
max_rss_kb            4176        2592        0.192% 0%         0.621   0.618 to 0.621    better (n=3)
load: median 5.13 (A) / 5.13 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### llvm sieve (load 5 at start)

```
A: llvm.go sieve 
B: llvm-sieve.xo sieve 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.13/5.13 5.13/5.13 5.13/5.13

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               111.2       108.2       1.874% 2.017%     0.973   0.949 to 0.992    better (n=3)
cpu_ms                170.6       107.3       0.785% 2.058%     0.629   0.616 to 0.642    better (n=3)
max_rss_kb            44528       21776       4.24% 0.037%      0.489   0.451 to 0.49     better (n=3)
load: median 5.13 (A) / 5.13 (B); loaded: 0 of 3 rounds (at or above 8.13; baseline 5.13 + 3.0, at least 6.0)
```

### llvm nbody (load 5 at start)

```
A: llvm.go nbody 
B: llvm-nbody.xo nbody 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.13/5.13 5.13/5.13 5.13/5.13

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               162.3       171.3       0.205% 0.124%     1.056   1.053 to 1.058    worse (n=3)
cpu_ms                161.7       170.5       0.211% 0.111%     1.055   1.052 to 1.057    worse (n=3)
max_rss_kb            4208        2592        0.19% 0%          0.616   0.616 to 0.618    better (n=3)
load: median 5.13 (A) / 5.13 (B); loaded: 0 of 3 rounds (at or above 8.13; baseline 5.13 + 3.0, at least 6.0)
```

### llvm sort (load 5 at start)

```
A: llvm.go sort 
B: llvm-sort.xo sort 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.13/5.13 5.13/5.13 4.8/5.13

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               269.1       220.8       0.823% 0.221%     0.821   0.817 to 0.828    better (n=3)
cpu_ms                285.2       219.8       1.621% 0.221%     0.771   0.763 to 0.786    better (n=3)
max_rss_kb            89136       55584       6.659% 0.043%     0.624   0.612 to 0.704    better (n=3)
load: median 5.13 (A) / 5.13 (B); loaded: 0 of 3 rounds (at or above 8.13; baseline 5.13 + 3.0, at least 6.0)
```

### llvm interp (load 4 at start)

```
A: llvm.go interp 
B: llvm-interp.xo interp 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.8/4.8 4.8/4.8 4.8/4.8

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               144.3       43.85       0.253% 5.464%     0.304   0.277 to 0.31     better (n=3)
cpu_ms                143.3       42.98       0.279% 5.684%     0.3     0.273 to 0.306    better (n=3)
max_rss_kb            4304        2592        1.487% 0.926%     0.602   0.591 to 0.613    better (n=3)
load: median 4.8 (A) / 4.8 (B); loaded: 0 of 3 rounds (at or above 7.8; baseline 4.8 + 3.0, at least 6.0)
```

### llvm collatz (load 4 at start)

```
A: llvm.go collatz 
B: llvm-collatz.xo collatz 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.8/4.8 4.8/4.8 4.8/4.8

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               469.2       431.0       0.301% 0.571%     0.919   0.909 to 0.923    better (n=3)
cpu_ms                468.6       430.2       0.312% 0.585%     0.918   0.908 to 0.922    better (n=3)
max_rss_kb            4192        2592        1.145% 0.309%     0.618   0.611 to 0.625    better (n=3)
load: median 4.8 (A) / 4.8 (B); loaded: 0 of 3 rounds (at or above 7.8; baseline 4.8 + 3.0, at least 6.0)
```

### llvm strings (load 4 at start)

```
A: llvm.go strings 
B: llvm-strings.xo strings 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.49/4.8 4.49/4.49 4.49/4.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               89.92       47.8        2.971% 4.011%     0.532   0.505 to 0.532    better (n=3)
cpu_ms                125.4       47.06       4.263% 4.269%     0.375   0.36 to 0.375     better (n=3)
max_rss_kb            25152       17632       8.27% 0%          0.701   0.654 to 0.773    better (n=3)
load: median 4.49 (A) / 4.49 (B); loaded: 0 of 3 rounds (at or above 7.8; baseline 4.8 + 3.0, at least 6.0)
```

### coll putget (load 4 at start)

```
A: coll.go putget 
B: coll.xo putget 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.49/4.49 4.49/4.49 4.49/4.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               50.42       30.23       1.055% 4.005%     0.6     0.598 to 0.635    better (n=3)
cpu_ms                49.87       29.39       1.079% 3.862%     0.589   0.587 to 0.624    too coarse (n=3)
max_rss_kb            8768        8608        0.274% 0%         0.982   0.978 to 0.984    better (n=3)
load: median 4.49 (A) / 4.49 (B); loaded: 0 of 3 rounds (at or above 7.49; baseline 4.49 + 3.0, at least 6.0)
```

### coll get (load 4 at start)

```
A: coll.go get 
B: coll.xo get 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.49/4.49 4.49/4.49 4.49/4.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.38       13.94       1.14% 3.214%      0.623   0.611 to 0.651    better (n=3)
cpu_ms                21.75       13.17       1.223% 2.821%     0.605   0.598 to 0.633    too coarse (n=3)
max_rss_kb            7024        7104        0.342% 0.225%     1.011   1.009 to 1.011    worse (n=3)
load: median 4.49 (A) / 4.49 (B); loaded: 0 of 3 rounds (at or above 7.49; baseline 4.49 + 3.0, at least 6.0)
```

### coll push (load 4 at start)

```
A: coll.go push 
B: coll.xo push 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.49/4.49 4.49/4.49 4.49/4.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               12.21       10.37       8.599% 1.764%     0.849   0.743 to 0.86     better (n=3)
cpu_ms                16.18       9.506       8.246% 1.462%     0.588   0.521 to 0.591    too coarse (n=3)
max_rss_kb            54304       56048       3.727% 0%         1.032   0.965 to 1.038    within noise (n=3)
load: median 4.49 (A) / 4.49 (B); loaded: 0 of 3 rounds (at or above 7.49; baseline 4.49 + 3.0, at least 6.0)
```

### coll update (load 4 at start)

```
A: coll.go update 
B: coll.xo update 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.49/4.49 4.49/4.49 4.49/4.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               30.44       19.52       0.907% 2.426%     0.641   0.614 to 0.651    better (n=3)
cpu_ms                30.0        18.83       1.063% 2.562%     0.628   0.597 to 0.639    too coarse (n=3)
max_rss_kb            8672        8608        1.476% 0%         0.993   0.98 to 1.009     within noise (n=3)
load: median 4.49 (A) / 4.49 (B); loaded: 0 of 3 rounds (at or above 7.49; baseline 4.49 + 3.0, at least 6.0)
```

### coll iter (load 4 at start)

```
A: coll.go iter 
B: coll.xo iter 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.49/4.49 4.49/4.49 4.49/4.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               21.86       6.118       1.366% 0.752%     0.28    0.276 to 0.287    better (n=3)
cpu_ms                21.28       5.39        1.189% 0.575%     0.253   0.247 to 0.256    too coarse (n=3)
max_rss_kb            6992        7104        0.915% 0.338%     1.016   1.004 to 1.016    worse (n=3)
load: median 4.49 (A) / 4.49 (B); loaded: 0 of 3 rounds (at or above 7.49; baseline 4.49 + 3.0, at least 6.0)
```

### coll copymut (load 4 at start)

```
A: coll.go copymut 
B: coll.xo copymut 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.49/4.49 4.49/4.49 4.49/4.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               14.37       9.571       1.938% 1.703%     0.666   0.664 to 0.688    better (n=3)
cpu_ms                14.38       8.82        1.687% 2.075%     0.613   0.603 to 0.634    too coarse (n=3)
max_rss_kb            10016       7152        1.677% 0%         0.714   0.697 to 0.721    better (n=3)
load: median 4.49 (A) / 4.49 (B); loaded: 0 of 3 rounds (at or above 7.49; baseline 4.49 + 3.0, at least 6.0)
```

### coll branch (load 4 at start)

```
A: coll.go branch 
B: coll.xo branch 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.49/4.49 4.49/4.49 4.49/4.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               216.6       5.103       10.65% 1.009%     0.024   0.02 to 0.025     better (n=3)
cpu_ms                339.5       4.276       10.32% 1.614%     0.013   0.011 to 0.014    better (n=3)
max_rss_kb            14912       7104        7.296% 0%         0.476   0.428 to 0.492    better (n=3)
load: median 4.49 (A) / 4.49 (B); loaded: 0 of 3 rounds (at or above 7.49; baseline 4.49 + 3.0, at least 6.0)
```

### conc spawn (load 4 at start)

```
A: conc.go spawn 
B: conc-spawn.xo spawn 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.49/4.49 4.49/4.49 4.49/4.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               52.34       23.19       3.486% 4.505%     0.443   0.436 to 0.457    better (n=3)
cpu_ms                171.2       32.9        3.847% 27.62%     0.192   0.137 to 0.24     better (n=3)
max_rss_kb            10768       6176        1.189% 8.031%     0.574   0.5 to 0.604      better (n=3)
load: median 4.49 (A) / 4.49 (B); loaded: 0 of 3 rounds (at or above 7.49; baseline 4.49 + 3.0, at least 6.0)
```

### conc fanout (load 4 at start)

```
A: conc.go fanout 
B: conc-fanout.xo fanout 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.49/4.49 4.49/4.49 4.49/4.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.73       15.6        6.798% 1.491%     0.686   0.637 to 0.753    better (n=3)
cpu_ms                62.54       15.0        6.48% 7.338%      0.24    0.226 to 0.295    too coarse (n=3)
max_rss_kb            11824       4544        0.812% 14.96%     0.384   0.384 to 0.497    better (n=3)
load: median 4.49 (A) / 4.49 (B); loaded: 0 of 3 rounds (at or above 7.49; baseline 4.49 + 3.0, at least 6.0)
```

### conc pingpong (load 4 at start)

```
A: conc.go pingpong 
B: conc-pingpong.xo pingpong 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.49/4.49 4.49/4.49 4.49/4.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               19.91       17.45       1.567% 0.607%     0.877   0.867 to 0.888    better (n=3)
cpu_ms                26.83       16.78       1.83% 0.486%      0.625   0.616 to 0.633    too coarse (n=3)
max_rss_kb            4192        2784        2.863% 0.575%     0.664   0.637 to 0.667    better (n=3)
load: median 4.49 (A) / 4.49 (B); loaded: 0 of 3 rounds (at or above 7.49; baseline 4.49 + 3.0, at least 6.0)
```

### conc mutex (load 4 at start)

```
A: conc.go mutex 
B: conc-mutex.xo mutex 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.49/4.49 4.49/4.49 4.49/4.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               23.35       6.322       1.968% 3.82%      0.271   0.264 to 0.291    better (n=3)
cpu_ms                99.34       5.59        3.84% 3.891%      0.056   0.055 to 0.063    too coarse (n=3)
max_rss_kb            4752        2688        0.842% 0.893%     0.566   0.56 to 0.58      better (n=3)
load: median 4.49 (A) / 4.49 (B); loaded: 0 of 3 rounds (at or above 7.49; baseline 4.49 + 3.0, at least 6.0)
```

### conc rwmutex (load 4 at start)

```
A: conc.go rwmutex 
B: conc-rwmutex.xo rwmutex 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.49/4.49 4.49/4.49 4.49/4.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               10.02       8.936       1.048% 1.354%     0.892   0.883 to 0.922    better (n=3)
cpu_ms                16.12       8.189       2.746% 1.661%     0.508   0.494 to 0.532    too coarse (n=3)
max_rss_kb            4784        2768        3.177% 0.867%     0.579   0.547 to 0.592    better (n=3)
load: median 4.49 (A) / 4.49 (B); loaded: 0 of 3 rounds (at or above 7.49; baseline 4.49 + 3.0, at least 6.0)
```

### conc pmap (load 4 at start)

```
A: conc.go pmap 
B: conc-pmap.xo pmap 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.49/4.49 4.49/4.49 4.49/4.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               54.59       56.99       0.629% 21.38%     1.044   1.01 to 1.451     worse (n=3)
cpu_ms                557.9       565.1       0.164% 0.286%     1.013   1.013 to 1.018    worse (n=3)
max_rss_kb            6688        5456        2.273% 0.88%      0.816   0.788 to 0.821    better (n=3)
load: median 4.49 (A) / 4.49 (B); loaded: 0 of 3 rounds (at or above 7.49; baseline 4.49 + 3.0, at least 6.0)
```


## Test (xo bench --ab, A = go test, B = xo test --std-module, 3 rounds)

Cold empties every cache before each round (Go with -count=1); edit appends one
function to the module under test before each round (Go -count=1). Copies of GOROOT
and the release's std.

### base64 (encoding/base64) cold (load 4 at start)

```
A: goroot/bin/go test -count=1 encoding/base64 
B: ~/xo-wt/rvg036-unpacked/xo-v0.36.0-darwin-arm64/bin/xo test --std-module=native/base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.34/4.34 4.34/4.96 4.96/4.96

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               2007        1135        0.329% 3.045%     0.566   0.552 to 0.59     better (n=3)
cpu_ms                11355       2680        0.569% 1.73%      0.236   0.235 to 0.241    better (n=3)
max_rss_kb            310816      355840      2.494% 0.25%      1.145   1.112 to 1.163    worse (n=3)
load: median 4.34 (A) / 4.96 (B); loaded: 0 of 3 rounds (at or above 7.37; baseline 4.37 + 3.0, at least 6.0)
```

### base64 (encoding/base64) edit (load 5 at start)

```
A: goroot/bin/go test -count=1 encoding/base64 
B: ~/xo-wt/rvg036-unpacked/xo-v0.36.0-darwin-arm64/bin/xo test --std-module=native/base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.34/5.34 5.34/5.34 5.34/5.34

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               317.1       258.5       4.54% 1.948%      0.815   0.771 to 0.816    better (n=3)
cpu_ms                966.5       389.9       4.213% 2.747%     0.403   0.387 to 0.404    better (n=3)
max_rss_kb            105264      195648      0.448% 0.969%     1.859   1.843 to 1.896    worse (n=3)
load: median 5.34 (A) / 5.34 (B); loaded: 0 of 3 rounds (at or above 8.34; baseline 5.34 + 3.0, at least 6.0)
```

### json (encoding/json) cold (load 5 at start)

```
A: goroot/bin/go test -count=1 encoding/json 
B: ~/xo-wt/rvg036-unpacked/xo-v0.36.0-darwin-arm64/bin/xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 8.43/8.43 8.16/10.39 10.28/10.39

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               3966        1333        1.506% 0.878%     0.336   0.327 to 0.337    better (n=3)
cpu_ms                24188       3143        0.885% 0.607%     0.13    0.128 to 0.131    better (n=3)
max_rss_kb            321408      375056      2.041% 0.215%     1.167   1.162 to 1.213    worse (n=3)
load: median 8.43 (A) / 10.39 (B); loaded: 3 of 3 rounds (at or above 8.34; baseline 5.34 + 3.0, at least 6.0)
warning: the machine was busy through the run (median load at or above 8.34): wall and cpu ratios are not reliable; measure with HEAVY_CLASS=measure (agent-tools.md Measuring) or --max-load
```

### json (encoding/json) edit (load 5 at start)

```
A: goroot/bin/go test -count=1 encoding/json 
B: ~/xo-wt/rvg036-unpacked/xo-v0.36.0-darwin-arm64/bin/xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.35/5.35 5.35/5.35 5.35/5.35

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               803.3       189.2       1.626% 1.193%     0.236   0.231 to 0.236    better (n=3)
cpu_ms                2736        328.1       1.702% 0.601%     0.12    0.116 to 0.121    better (n=3)
max_rss_kb            163232      213856      0.456% 2.42%      1.31    1.3 to 1.359      worse (n=3)
load: median 5.35 (A) / 5.35 (B); loaded: 0 of 3 rounds (at or above 8.64; baseline 5.64 + 3.0, at least 6.0)
```


## HTTP server (bench/perf/jitter, linux/arm64 offload host)

- host load 3.59 3.98 3.45 1/310 3837029
- release-vs-go-http rvg80962: 9c92dc87031 on Linux aarch64, 12 CPUs, go1.26.1
- xo xo 0.36.0 linux/arm64; base xo 0.35.0 linux/arm64
- loadgen flags: -c C -rate R -d 10s -warmup 2s -awake (default on in open loop); servers on CPUs 0-5, loadgen on 6-11
- Go's /alloc rows are its /alloc-hand (json.Marshal written by hand, as Xo's derive encodes).
- Rounds: 5 per row; each cell is the median, its range in brackets; CPU is the server's CPU microseconds per request.

| row | server | req/s | p50 us | p99 us | p99.9 us | max us | CPU us/req | errors |
|---|---|---|---|---|---|---|---|---|
| /alloc c32 | xo | 164,777 [164,543 to 165,216] | 178 [177 to 179] | 466 [465 to 469] | 1,258 [1,245 to 1,300] | 6,323 [6,260 to 6,910] | 34.8 [34.7 to 34.9] | 0 [0 to 0] |
| /alloc c32 | base | 171,724 [170,118 to 172,301] | 171 [170 to 171] | 455 [454 to 471] | 1,172 [1,137 to 1,196] | 6,193 [5,461 to 8,494] | 33.0 [32.8 to 33.3] | 0 [0 to 0] |
| /alloc c32 | go | 27,909 [27,703 to 27,963] | 491 [484 to 496] | 5,520 [5,454 to 5,537] | 8,039 [8,014 to 8,070] | 13,959 [13,563 to 14,342] | 118.6 [117.5 to 119.4] | 0 [0 to 0] |
| /alloc c64 open 10000/s | xo | 10,000 [10,000 to 10,000] | 79 [79 to 80] | 111 [110 to 122] | 138 [132 to 145] | 211 [194 to 467] | 52.7 [52.6 to 53.5] | 0 [0 to 0] |
| /alloc c64 open 10000/s | base | 10,000 [10,000 to 10,000] | 77 [77 to 77] | 110 [108 to 111] | 137 [131 to 139] | 188 [176 to 208] | 50.0 [49.6 to 50.5] | 0 [0 to 0] |
| /alloc c64 open 10000/s | go | 10,000 [10,000 to 10,000] | 100 [99 to 101] | 392 [359 to 394] | 507 [463 to 512] | 864 [823 to 1,092] | 122.2 [114.5 to 122.4] | 0 [0 to 0] |
| /alloc c64 open 20000/s | xo | 20,000 [20,000 to 20,000] | 92 [92 to 92] | 145 [145 to 146] | 168 [166 to 170] | 289 [223 to 1,295] | 58.2 [58.1 to 58.7] | 0 [0 to 0] |
| /alloc c64 open 20000/s | base | 20,000 [20,000 to 20,000] | 89 [89 to 90] | 140 [139 to 141] | 164 [163 to 165] | 233 [209 to 254] | 54.7 [54.6 to 55.0] | 0 [0 to 0] |
| /alloc c64 open 20000/s | go | 20,000 [20,000 to 20,000] | 192 [184 to 195] | 683 [645 to 692] | 991 [872 to 1,197] | 2,844 [1,737 to 4,651] | 128.1 [125.7 to 129.4] | 0 [0 to 0] |
| /alloc c256 | xo | 174,004 [172,382 to 175,986] | 1,276 [1,267 to 1,287] | 5,292 [5,236 to 5,350] | 6,528 [6,449 to 6,751] | 10,791 [10,498 to 11,461] | 34.4 [34.1 to 34.7] | 0 [0 to 0] |
| /alloc c256 | base | 183,284 [181,749 to 185,962] | 1,214 [1,205 to 1,222] | 5,119 [4,986 to 5,167] | 6,298 [6,207 to 6,483] | 9,637 [8,947 to 11,364] | 32.7 [32.2 to 33.0] | 0 [0 to 0] |
| /alloc c256 | go | 56,874 [56,801 to 57,638] | 397 [377 to 402] | 38,207 [37,848 to 38,425] | 57,138 [56,347 to 57,630] | 109,714 [95,575 to 113,917] | 81.8 [80.9 to 82.1] | 0 [0 to 0] |
| /hello c32 | xo | 357,623 [342,627 to 362,307] | 85 [84 to 86] | 221 [219 to 242] | 322 [313 to 1,145] | 3,734 [3,498 to 5,823] | 7.3 [7.2 to 7.3] | 0 [0 to 0] |
| /hello c32 | base | 359,478 [357,641 to 360,095] | 85 [84 to 86] | 221 [220 to 221] | 320 [315 to 325] | 4,127 [3,818 to 4,583] | 7.2 [7.1 to 7.2] | 0 [0 to 0] |
| /hello c32 | go | 277,906 [277,205 to 280,974] | 101 [99 to 101] | 637 [635 to 638] | 851 [849 to 855] | 3,519 [2,984 to 4,434] | 12.4 [12.2 to 12.4] | 0 [0 to 0] |
| /hello c256 | xo | 897,669 [892,921 to 904,356] | 255 [253 to 256] | 820 [805 to 839] | 1,779 [1,693 to 1,824] | 7,858 [6,908 to 9,334] | 5.9 [5.8 to 5.9] | 0 [0 to 0] |
| /hello c256 | base | 908,926 [902,372 to 911,423] | 252 [251 to 253] | 802 [799 to 811] | 1,782 [1,682 to 1,841] | 9,028 [7,266 to 12,017] | 5.8 [5.8 to 5.9] | 0 [0 to 0] |
| /hello c256 | go | 488,227 [484,257 to 495,743] | 307 [303 to 307] | 2,600 [2,566 to 2,697] | 3,769 [3,523 to 4,508] | 8,448 [7,553 to 10,053] | 10.0 [9.9 to 10.1] | 0 [0 to 0] |

- /alloc c32, xo/go medians: req/s 5.904, p50 us 0.363, p99 us 0.084, p99.9 us 0.156, max us 0.453, CPU us/req 0.293
- /alloc c32, xo/base medians: req/s 0.960, p50 us 1.041, p99 us 1.024, p99.9 us 1.073, max us 1.021, CPU us/req 1.055
- /alloc c64 open 10000/s, xo/go medians: req/s 1.000, p50 us 0.790, p99 us 0.283, p99.9 us 0.272, max us 0.244, CPU us/req 0.431
- /alloc c64 open 10000/s, xo/base medians: req/s 1.000, p50 us 1.026, p99 us 1.009, p99.9 us 1.007, max us 1.122, CPU us/req 1.054
- /alloc c64 open 20000/s, xo/go medians: req/s 1.000, p50 us 0.479, p99 us 0.212, p99.9 us 0.170, max us 0.102, CPU us/req 0.455
- /alloc c64 open 20000/s, xo/base medians: req/s 1.000, p50 us 1.034, p99 us 1.036, p99.9 us 1.024, max us 1.240, CPU us/req 1.064
- /alloc c256, xo/go medians: req/s 3.059, p50 us 3.214, p99 us 0.139, p99.9 us 0.114, max us 0.098, CPU us/req 0.421
- /alloc c256, xo/base medians: req/s 0.949, p50 us 1.051, p99 us 1.034, p99.9 us 1.037, max us 1.120, CPU us/req 1.054
- /hello c32, xo/go medians: req/s 1.287, p50 us 0.842, p99 us 0.347, p99.9 us 0.378, max us 1.061, CPU us/req 0.587
- /hello c32, xo/base medians: req/s 0.995, p50 us 1.000, p99 us 1.000, p99.9 us 1.006, max us 0.905, CPU us/req 1.011
- /hello c256, xo/go medians: req/s 1.839, p50 us 0.831, p99 us 0.315, p99.9 us 0.472, max us 0.930, CPU us/req 0.589
- /hello c256, xo/base medians: req/s 0.988, p50 us 1.012, p99 us 1.022, p99.9 us 0.998, max us 0.870, CPU us/req 1.011
