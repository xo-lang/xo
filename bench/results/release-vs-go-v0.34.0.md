# Xo against Go: xo 0.34.0 darwin/arm64 (2026-10-29)

The release comparison (toolchain.md "Cutting a release" step 7) on the tag
v0.34.0 (f1a3c02c7), the tag's tree and xo, scripts/release-vs-go.sh as of
97a348e0b: std's variants are built after the cold row and the edit row is
the std-record loader (its last column says which path the edit took), xo's
builds run with the whole-build run cache off. Quiet windows (load under 6
at every start). The comparison against the previous release's report
(release-vs-go-v0.33.0.md; 0.33.1 was a patch from the 0.33.0 branch with no
report of its own) follows the script's report: each run row's B/A ratio
(Xo over Go) old against new, a row worse by more than 3% flagged.

## Against 0.33.0

Every run started at load 4 or 5. Wall ratios against Go: one row worse than
0.33.0's by more than 3%, conc rwmutex 0.869 to 0.897 (+3.2%, inside its
spread); better: interp 0.500 to 0.291, pingpong 1.196 to 0.897, spawn 0.463
to 0.428, pmap 1.086 to 1.010, fanout 0.704 to 0.667, mutex 0.284 to 0.271,
iter 0.277 to 0.264. Rows above Go: nbody 1.040 (0.33.0: 1.054) and pmap
1.010 (1.086), both closer than before. Flagged below the wall rows: peak RSS
up 5 to 9% on sieve (0.392 to 0.413), sort (0.417 to 0.455) and fanout (0.376
to 0.409), one cause to look for (separately compiled programs link std's
store whole: the binaries are 1.78 MB against 0.33.0's 1.60, and a larger
image touches more pages; dead stripping of the store is 0388 stage 4); cpu
rows for sieve, nbody and strings move 2 to 3 ms on 10 ms granularity, and
coll get's cpu 0.5 to 1.0 is 5 ms against 10 ms of the same granularity.
Compile: xo's edit builds 60 to 70 ms (coll 130) against Go's 100 to 110,
the std-record loader path on every row (0.33.0's 210 to 230 were
whole-program builds, gotchas/sepcomp-default-check-needs-std-variants.md);
cold 850 to 1030 against Go's 1440; the second and warm rows are compiles
through the back-end caches now (the run cache is off for the rows), not the
run cache's 10 to 20 ms.

| workload | metric | 0.33.0 B/A | pre-cut B/A | change |
|---|---|---|---|---|
| llvm fib | wall_ms | 0.957 | 0.966 | +0.9% |
| llvm fib | cpu_ms | 0.955 | 0.955 | +0.0% |
| llvm fib | max_rss_kb | 0.612 | 0.627 | +2.5% |
| llvm sieve | wall_ms | 0.926 | 0.924 | -0.2% |
| llvm sieve | cpu_ms | 0.606 | 0.625 | +3.1% WORSE |
| llvm sieve | max_rss_kb | 0.392 | 0.413 | +5.4% WORSE |
| llvm nbody | wall_ms | 1.054 | 1.040 | -1.3% |
| llvm nbody | cpu_ms | 1.032 | 1.067 | +3.4% WORSE |
| llvm nbody | max_rss_kb | 0.611 | 0.621 | +1.6% |
| llvm sort | wall_ms | 0.806 | 0.815 | +1.1% |
| llvm sort | cpu_ms | 0.778 | 0.778 | +0.0% |
| llvm sort | max_rss_kb | 0.417 | 0.455 | +9.1% WORSE |
| llvm interp | wall_ms | 0.500 | 0.291 | -41.8% |
| llvm interp | cpu_ms | 0.429 | 0.250 | -41.7% |
| llvm interp | max_rss_kb | 0.606 | 0.623 | +2.8% |
| llvm collatz | wall_ms | 0.960 | 0.965 | +0.5% |
| llvm collatz | cpu_ms | 0.957 | 0.959 | +0.2% |
| llvm collatz | max_rss_kb | 0.612 | 0.624 | +2.0% |
| llvm strings | wall_ms | 0.462 | 0.458 | -0.9% |
| llvm strings | cpu_ms | 0.250 | 0.273 | +9.2% WORSE |
| llvm strings | max_rss_kb | 0.644 | 0.613 | -4.8% |
| coll putget | wall_ms | 0.581 | 0.579 | -0.3% |
| coll putget | cpu_ms | 0.500 | 0.500 | +0.0% |
| coll putget | max_rss_kb | 0.832 | 0.833 | +0.1% |
| coll get | wall_ms | 0.602 | 0.599 | -0.5% |
| coll get | cpu_ms | 0.500 | 1.000 | +100.0% WORSE |
| coll get | max_rss_kb | 0.969 | 0.971 | +0.2% |
| coll push | wall_ms | 0.743 | 0.735 | -1.1% |
| coll push | cpu_ms | 0.000 | 0.000 | +0.0% |
| coll push | max_rss_kb | 0.932 | 0.877 | -5.9% |
| coll update | wall_ms | 0.613 | 0.601 | -2.0% |
| coll update | cpu_ms | 0.500 | 0.500 | +0.0% |
| coll update | max_rss_kb | 0.835 | 0.833 | -0.2% |
| coll iter | wall_ms | 0.277 | 0.264 | -4.7% |
| coll iter | cpu_ms | 0.000 | 0.000 | +0.0% |
| coll iter | max_rss_kb | 0.967 | 0.975 | +0.8% |
| coll copymut | wall_ms | 0.661 | 0.675 | +2.1% |
| coll copymut | cpu_ms | 0.000 | 0.000 | +0.0% |
| coll copymut | max_rss_kb | 0.687 | 0.684 | -0.4% |
| coll branch | wall_ms | 0.025 | 0.025 | +0.0% |
| coll branch | cpu_ms | 0.000 | 0.000 | +0.0% |
| coll branch | max_rss_kb | 0.452 | 0.456 | +0.9% |
| conc spawn | wall_ms | 0.463 | 0.428 | -7.6% |
| conc spawn | cpu_ms | 0.133 | 0.133 | +0.0% |
| conc spawn | max_rss_kb | 0.535 | 0.504 | -5.8% |
| conc fanout | wall_ms | 0.704 | 0.667 | -5.3% |
| conc fanout | cpu_ms | 0.200 | 0.200 | +0.0% |
| conc fanout | max_rss_kb | 0.376 | 0.409 | +8.8% WORSE |
| conc pingpong | wall_ms | 1.196 | 0.897 | -25.0% |
| conc pingpong | cpu_ms | 2.000 | 1.000 | -50.0% |
| conc pingpong | max_rss_kb | 0.626 | 0.638 | +1.9% |
| conc mutex | wall_ms | 0.284 | 0.271 | -4.6% |
| conc mutex | cpu_ms | 0.000 | 0.000 | +0.0% |
| conc mutex | max_rss_kb | 0.542 | 0.555 | +2.4% |
| conc rwmutex | wall_ms | 0.869 | 0.897 | +3.2% WORSE |
| conc rwmutex | cpu_ms | 0.000 | 1.000 | +0.0% |
| conc rwmutex | max_rss_kb | 0.541 | 0.554 | +2.4% |
| conc pmap | wall_ms | 1.086 | 1.010 | -7.0% |
| conc pmap | cpu_ms | 1.093 | 1.018 | -6.9% |
| conc pmap | max_rss_kb | 0.789 | 0.794 | +0.6% |

flagged (worse than 0.33.0 by more than 3%): 8
   ('llvm sieve', 'cpu_ms', 0.606, 0.625)
   ('llvm sieve', 'max_rss_kb', 0.392, 0.413)
   ('llvm nbody', 'cpu_ms', 1.032, 1.067)
   ('llvm sort', 'max_rss_kb', 0.417, 0.455)
   ('llvm strings', 'cpu_ms', 0.25, 0.273)
   ('coll get', 'cpu_ms', 0.5, 1.0)
   ('conc fanout', 'max_rss_kb', 0.376, 0.409)
   ('conc rwmutex', 'wall_ms', 0.869, 0.897)


## The peak RSS rows, directly (2026-10-29)

sort, xo bench --ab, 20 rounds, load 2.6 to 2.8, nothing else running: the
0.33.0 release xo's build against the 0.34.0 xo's, peak RSS 34592 KB against
34592 KB (B/A 1.0, interval 1.0 to 1.0), wall 0.997; and the 0.34.0 xo's
whole-program build (XO_SEPCOMP=0) against its default, 34600 against 34592
KB (1.0). Xo's peak RSS did not move between the releases or between the
link paths; the ratio's change (0.417 to 0.455) is Go's side of the pair
(its sort at about 83 MB in the 0.33.0 run and 76 MB in this one, a GC
pacing difference between runs), and the sieve and fanout rows move the
same way. Not an Xo regression; the store-link suspicion above is withdrawn.

## The run (Xo against Go: xo 0.34.0 darwin/arm64 (2026-10-29))

scripts/release-vs-go.sh on Darwin arm64, 24 GB,
Go go1.26.1. Build times are wall milliseconds of one build; peak MB is the
build's largest resident set (cold / warm / edit). Warm is the third unchanged build
(xo's parts pack is written on the second and read from the third); second is the second.
The edit's path is what xo's cache statistics say the edit build did: separate (against
std's variants, built after the cold row as a user's second build would have them),
with the std records, or whole (the whole program).
xo's builds run with the whole-build run cache off (XO_BIN_CACHE=off), so the second and
warm rows are compiles through the back-end caches, not a cached result (before 2026-10-29
they could be the run cache's 10 to 20 ms).
Size is the default build's binary.

## Compile and size

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB | edit's path |
|---|---|---|---|---|---|---|---|---|
| llvm | go | 1450 | 40 | 40 | 100 | 314 / 26 / 68 | 2.39 | - |
| coll | go | 1440 | 40 | 40 | 110 | 302 / 26 / 68 | 2.43 | - |
| conc | go | 1440 | 40 | 40 | 100 | 310 / 25 / 67 | 2.40 | - |
| llvm-fib | xo | 870 | 70 | 60 | 70 | 405 / 125 / 134 | 1.78 | separate, std records |
| llvm-sieve | xo | 860 | 70 | 70 | 60 | 411 / 126 / 130 | 1.78 | separate, std records |
| llvm-nbody | xo | 860 | 80 | 70 | 70 | 410 / 129 / 133 | 1.78 | separate, std records |
| llvm-sort | xo | 850 | 70 | 70 | 70 | 415 / 129 / 134 | 1.78 | separate, std records |
| llvm-interp | xo | 870 | 80 | 70 | 70 | 404 / 127 / 131 | 1.78 | separate, std records |
| llvm-collatz | xo | 870 | 70 | 60 | 60 | 410 / 126 / 130 | 1.78 | separate, std records |
| llvm-strings | xo | 870 | 70 | 70 | 70 | 406 / 129 / 130 | 1.78 | separate, std records |
| coll | xo | 890 | 140 | 120 | 130 | 419 / 144 / 148 | 1.86 | separate, std records |
| conc-spawn | xo | 1030 | 70 | 70 | 70 | 410 / 133 / 132 | 1.88 | separate, std records |
| conc-fanout | xo | 1000 | 70 | 60 | 70 | 405 / 133 / 138 | 1.90 | separate, std records |
| conc-pingpong | xo | 1010 | 70 | 70 | 70 | 410 / 132 / 138 | 1.91 | separate, std records |
| conc-mutex | xo | 1000 | 70 | 70 | 70 | 407 / 134 / 133 | 1.94 | separate, std records |
| conc-rwmutex | xo | 1000 | 70 | 70 | 70 | 409 / 131 / 137 | 1.95 | separate, std records |
| conc-pmap | xo | 990 | 80 | 70 | 70 | 406 / 132 / 136 | 1.90 | separate, std records |

The Go column builds one binary per group (llvm, coll, conc); the Xo
programs are one per workload except coll.

## Run (xo bench --ab, A = Go, B = Xo, 10 rounds)

### llvm fib (load 5 at start)

```
A: llvm.go fib 
B: llvm-fib.xo fib 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.94/5.94 5.94/5.55 5.55/5.55 5.55/5.55 5.55/5.55 5.55/5.55 5.55/5.55 5.55/5.55 5.55/5.55 5.55/5.55

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               228.5       220.8       1.269% 2.036%     0.966   0.959 to 0.99     better (n=10)
cpu_ms                220.0       210.0       0% 4.762%         0.955   0.955 to 1.0      within noise (n=10)
max_rss_kb            4208        2640        0.665% 0%         0.627   0.624 to 0.632    better (n=10)
load: median 5.55 (A) / 5.55 (B); loaded: 0 of 10 rounds (at or above 8.94; baseline 5.94 + 3.0, at least 6.0)
```

### llvm sieve (load 5 at start)

```
A: llvm.go sieve 
B: llvm-sieve.xo sieve 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.55/5.55 5.55/5.55 5.55/5.55 5.55/5.55 5.26/5.26 5.26/5.26 5.26/5.26 5.26/5.26 5.26/5.26 5.26/5.26

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               112.2       103.6       1.871% 10.99%     0.924   0.902 to 1.025    within noise (n=10)
cpu_ms                160.0       100.0       10.94% 17.5%      0.625   0.543 to 0.688    better (n=10)
max_rss_kb            43984       18160       5.602% 0%         0.413   0.397 to 0.42     better (n=10)
load: median 5.26 (A) / 5.26 (B); loaded: 0 of 10 rounds (at or above 8.55; baseline 5.55 + 3.0, at least 6.0)
```

### llvm nbody (load 5 at start)

```
A: llvm.go nbody 
B: llvm-nbody.xo nbody 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.26/5.26 5.26/5.26 5.26/5.26 5.26/5.26 5.26/5.26 5.26/5.26 5.26/5.26 5.26/5.26 5.26/5.26 5.26/5.26

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               161.7       168.2       0.134% 0.52%      1.04    1.039 to 1.044    worse (n=10)
cpu_ms                150.0       160.0       0% 0%             1.067   1.067 to 1.067    worse (n=10)
max_rss_kb            4264        2648        0.657% 1.511%     0.621   0.618 to 0.631    better (n=10)
load: median 5.26 (A) / 5.26 (B); loaded: 0 of 10 rounds (at or above 8.26; baseline 5.26 + 3.0, at least 6.0)
```

### llvm sort (load 4 at start)

```
A: llvm.go sort 
B: llvm-sort.xo sort 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.92/4.92 4.92/4.92 4.92/4.92 4.92/4.92 4.92/4.92 4.92/4.92 4.92/4.92 4.92/4.92 4.92/4.92 4.92/4.85

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               265.2       216.2       0.385% 0.347%     0.815   0.812 to 0.818    better (n=10)
cpu_ms                270.0       210.0       0% 0%             0.778   0.778 to 0.778    better (n=10)
max_rss_kb            76088       34640       12.08% 0.035%     0.455   0.432 to 0.538    better (n=10)
load: median 4.92 (A) / 4.92 (B); loaded: 0 of 10 rounds (at or above 7.92; baseline 4.92 + 3.0, at least 6.0)
```

### llvm interp (load 4 at start)

```
A: llvm.go interp 
B: llvm-interp.xo interp 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.85/4.85 4.85/4.85 4.85/4.85 4.85/4.85 4.85/4.85 4.85/4.85 4.85/4.85 4.85/4.85 4.85/4.85 4.85/4.85

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               145.5       42.3        1.072% 5.963%     0.291   0.28 to 0.31      better (n=10)
cpu_ms                140.0       35.0        0% 28.57%         0.25    0.214 to 0.286    better (n=10)
max_rss_kb            4240        2640        2.075% 0%         0.623   0.618 to 0.632    better (n=10)
load: median 4.85 (A) / 4.85 (B); loaded: 0 of 10 rounds (at or above 7.85; baseline 4.85 + 3.0, at least 6.0)
```

### llvm collatz (load 4 at start)

```
A: llvm.go collatz 
B: llvm-collatz.xo collatz 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.85/4.85 4.85/4.85 4.54/4.54 4.54/4.54 4.54/4.54 4.54/4.54 4.54/4.54 4.26/4.26 4.26/4.26 4.26/4.26

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               494.7       477.2       0.074% 0.478%     0.965   0.957 to 0.967    better (n=10)
cpu_ms                490.0       470.0       0% 0%             0.959   0.959 to 0.959    better (n=10)
max_rss_kb            4232        2640        1.985% 0%         0.624   0.619 to 0.635    better (n=10)
load: median 4.54 (A) / 4.54 (B); loaded: 0 of 10 rounds (at or above 7.85; baseline 4.85 + 3.0, at least 6.0)
```

### llvm strings (load 4 at start)

```
A: llvm.go strings 
B: llvm-strings.xo strings 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.26/4.26 4.26/4.26 4.26/4.26 4.26/4.26 4.26/4.26 4.26/4.26 4.26/4.26 4.26/4.26 4.26/4.26 4.26/4.26

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               89.82       41.15       2.46% 4.022%      0.458   0.45 to 0.475     better (n=10)
cpu_ms                110.0       30.0        6.818% 0%         0.273   0.25 to 0.318     better (n=10)
max_rss_kb            24992       15328       13.86% 0.313%     0.613   0.557 to 0.657    better (n=10)
load: median 4.26 (A) / 4.26 (B); loaded: 0 of 10 rounds (at or above 7.26; baseline 4.26 + 3.0, at least 6.0)
```

### coll putget (load 4 at start)

```
A: coll.go putget 
B: coll.xo putget 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.26/4.26 4.26/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               49.77       28.8        1.005% 1.327%     0.579   0.575 to 0.59     better (n=10)
cpu_ms                40.0        20.0        0% 0%             0.5     0.5 to 0.5        too coarse (n=10)
max_rss_kb            8776        7312        2.461% 0%         0.833   0.829 to 0.856    better (n=10)
load: median 4.0 (A) / 4.0 (B); loaded: 0 of 10 rounds (at or above 7.26; baseline 4.26 + 3.0, at least 6.0)
```

### coll get (load 4 at start)

```
A: coll.go get 
B: coll.xo get 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.32       13.38       1.305% 2.347%     0.599   0.587 to 0.609    better (n=10)
cpu_ms                10.0        10.0        100.0% 0%         1.0     0.5 to 1.0        too coarse (n=10)
max_rss_kb            7136        6928        1.177% 0.346%     0.971   0.964 to 0.981    better (n=10)
load: median 4.0 (A) / 4.0 (B); loaded: 0 of 10 rounds (at or above 7.0; baseline 4.0 + 3.0, at least 6.0)
```

### coll push (load 4 at start)

```
A: coll.go push 
B: coll.xo push 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               11.97       8.792       2.936% 3.424%     0.735   0.711 to 0.749    better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            58160       51008       16.08% 0.047%     0.877   0.728 to 0.91     better (n=10)
load: median 4.0 (A) / 4.0 (B); loaded: 0 of 10 rounds (at or above 7.0; baseline 4.0 + 3.0, at least 6.0)
```

### coll update (load 4 at start)

```
A: coll.go update 
B: coll.xo update 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               30.69       18.44       1.507% 2.139%     0.601   0.591 to 0.611    better (n=10)
cpu_ms                20.0        10.0        0% 0%             0.5     0.5 to 0.5        too coarse (n=10)
max_rss_kb            8792        7320        0.819% 0.219%     0.833   0.828 to 0.837    better (n=10)
load: median 4.0 (A) / 4.0 (B); loaded: 0 of 10 rounds (at or above 7.0; baseline 4.0 + 3.0, at least 6.0)
```

### coll iter (load 4 at start)

```
A: coll.go iter 
B: coll.xo iter 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               21.99       5.811       0.78% 1.996%      0.264   0.262 to 0.269    better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            7104        6928        1.52% 0.577%      0.975   0.965 to 0.984    better (n=10)
load: median 4.0 (A) / 4.0 (B); loaded: 0 of 10 rounds (at or above 7.0; baseline 4.0 + 3.0, at least 6.0)
```

### coll copymut (load 4 at start)

```
A: coll.go copymut 
B: coll.xo copymut 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               14.08       9.509       1.08% 1.488%      0.675   0.667 to 0.682    better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            10144       6936        4.456% 0.692%     0.684   0.668 to 0.711    better (n=10)
load: median 4.0 (A) / 4.0 (B); loaded: 0 of 10 rounds (at or above 7.0; baseline 4.0 + 3.0, at least 6.0)
```

### coll branch (load 4 at start)

```
A: coll.go branch 
B: coll.xo branch 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               198.1       4.995       3.444% 4.615%     0.025   0.024 to 0.026    better (n=10)
cpu_ms                290.0       0           2.586% 0%         0       0 to 0            better (n=10)
max_rss_kb            15208       6928        9.942% 0%         0.456   0.42 to 0.477     better (n=10)
load: median 4.0 (A) / 4.0 (B); loaded: 0 of 10 rounds (at or above 7.0; baseline 4.0 + 3.0, at least 6.0)
```

### conc spawn (load 4 at start)

```
A: conc.go spawn 
B: conc-spawn.xo spawn 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               49.1        21.0        2.71% 2.152%      0.428   0.42 to 0.444     better (n=10)
cpu_ms                150.0       20.0        0% 50.0%          0.133   0.067 to 0.133    better (n=10)
max_rss_kb            11040       5568        3.696% 17.03%     0.504   0.459 to 0.561    better (n=10)
load: median 4.0 (A) / 4.0 (B); loaded: 0 of 10 rounds (at or above 7.0; baseline 4.0 + 3.0, at least 6.0)
```

### conc fanout (load 4 at start)

```
A: conc.go fanout 
B: conc-fanout.xo fanout 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.08       14.72       11.23% 0.902%     0.667   0.608 to 0.706    better (n=10)
cpu_ms                50.0        10.0        15.0% 0%          0.2     0.167 to 0.2      too coarse (n=10)
max_rss_kb            11608       4744        2.481% 3.879%     0.409   0.394 to 0.419    better (n=10)
load: median 4.0 (A) / 4.0 (B); loaded: 0 of 10 rounds (at or above 7.0; baseline 4.0 + 3.0, at least 6.0)
```

### conc pingpong (load 4 at start)

```
A: conc.go pingpong 
B: conc-pingpong.xo pingpong 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               19.02       17.06       1.67% 1.501%      0.897   0.884 to 0.909    better (n=10)
cpu_ms                10.0        10.0        0% 0%             1.0     1.0 to 1.0        too coarse (n=10)
max_rss_kb            4288        2736        2.332% 0.877%     0.638   0.63 to 0.647     better (n=10)
load: median 4.0 (A) / 4.0 (B); loaded: 0 of 10 rounds (at or above 7.0; baseline 4.0 + 3.0, at least 6.0)
```

### conc mutex (load 4 at start)

```
A: conc.go mutex 
B: conc-mutex.xo mutex 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.41       6.08        1.873% 1.493%     0.271   0.268 to 0.277    better (n=10)
cpu_ms                80.0        0           9.375% 0%         0       0 to 0            too coarse (n=10)
max_rss_kb            4840        2688        1.901% 1.637%     0.555   0.553 to 0.574    better (n=10)
load: median 4.0 (A) / 4.0 (B); loaded: 0 of 10 rounds (at or above 7.0; baseline 4.0 + 3.0, at least 6.0)
```

### conc rwmutex (load 4 at start)

```
A: conc.go rwmutex 
B: conc-rwmutex.xo rwmutex 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               9.268       8.313       1.829% 3.263%     0.897   0.875 to 0.915    better (n=10)
cpu_ms                0           0           0% 0%             1.0     1.0 to 1.0        too coarse (n=10)
max_rss_kb            4880        2704        0.902% 1.627%     0.554   0.549 to 0.564    better (n=10)
load: median 4.0 (A) / 4.0 (B); loaded: 0 of 10 rounds (at or above 7.0; baseline 4.0 + 3.0, at least 6.0)
```

### conc pmap (load 4 at start)

```
A: conc.go pmap 
B: conc-pmap.xo pmap 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0 4.0/4.0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               53.97       54.54       0.307% 0.835%     1.01    1.004 to 1.019    worse (n=10)
cpu_ms                550.0       560.0       0% 0%             1.018   1.018 to 1.018    worse (n=10)
max_rss_kb            6848        5440        0.818% 0.588%     0.794   0.788 to 0.799    better (n=10)
load: median 4.0 (A) / 4.0 (B); loaded: 0 of 10 rounds (at or above 7.0; baseline 4.0 + 3.0, at least 6.0)
```

