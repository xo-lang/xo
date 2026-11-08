# Xo against Go: main before the 0.40.0 cut (2026-11-05)

A pre-cut run of scripts/release-vs-go.sh (Xo Lang 2, 2026-11-05) on main
b8814fa12 with that commit's xo, Darwin arm64, 24 GB, Go go1.26.1; the HTTP
section on the linux/arm64 offload host against the 0.39.0 release (its
linux/arm64 xo builds the base servers), the amd64 section on the
linux/amd64 offload host against the 0.39.0 release (its darwin/arm64
archive builds the base). The report below is the script's. Before it: each
run and test row's B/A (Xo over Go) against release-vs-go-v0.39.0-precut.md,
a row worse by more than 3% flagged and checked with a direct 0.39.0
against main `xo bench --ab` (9 rounds, darwin/arm64).

## Against 0.39.0's pre-cut

| row | 0.39.0 pre-cut Xo/Go | b8814fa12 Xo/Go | check: 0.39.0 against main, B/A |
|---|---|---|---|
| llvm fib | 1.351 | 0.988 | |
| llvm sieve | 0.708 | 0.940 flagged | darwin/arm64 1.02 (0.877 to 1.138), within noise, 9 rounds |
| llvm nbody | 1.054 | 1.037 | |
| llvm sort | 0.825 | 0.819 | |
| llvm interp | 0.298 | 0.296 | |
| llvm collatz | 0.971 | 0.917 | |
| llvm strings | 0.497 | 0.485 | |
| coll putget | 0.627 | 0.671 flagged | darwin/arm64 0.983 (0.97 to 1.022), within noise, 9 rounds |
| coll get | 0.608 | 0.599 | |
| coll push | 0.689 | 0.687 | |
| coll update | 0.610 | 0.633 flagged | darwin/arm64 1.037 (1.019 to 1.048) worse, 9 rounds; pad sweep below |
| coll iter | 0.270 | 0.265 | |
| coll copymut | 0.680 | 0.669 | |
| coll branch | 0.025 | 0.025 | |
| conc spawn | 0.416 | 0.463 flagged | darwin/arm64 1.191 (1.021 to 1.253) worse, CPU 2.144, 9 rounds; decision 0514, fixed by 0518 (59ce17584): 0.973 within noise; see below |
| conc fanout | 0.580 | 0.551 | |
| conc pingpong | 0.921 | 0.999 flagged | darwin/arm64 0.997 (0.987 to 1.047), within noise, 9 rounds |
| conc mutex | 0.320 | 0.319 | |
| conc rwmutex | 1.001 | 0.981 | |
| conc pmap | 0.971 | 0.983 | |
| base64 test cold | 0.670 | 0.669 | |
| base64 test edit | 0.824 | 0.789 | |
| json test cold | 0.495 | 0.367 | |
| json test edit | 0.303 | 0.338 flagged | the same section with the 0.39.0 release's xo today: 0.327 (0.31 to 0.341) against main's 0.338 (0.335 to 0.344), overlapping: not slower |

HTTP (linux/arm64, 4 pads): main against 0.39.0 is 0.997 to 1.002 of its
req/s on every row, "placement" by the pad sweep (no row slower or faster);
against Go, /hello 1.284 (c32) and 1.798 (c256) of its req/s, /alloc 5.983
(c32) and 3.130 (c256).

Note (2026-11-08): release-vs-go-amd64.sh ran these binaries from /mnt/c,
where drvfs adds about 6 ms of exec to every run (fib32 13.8 against 7.9 ms
from ext4), more for Go's larger binary, so the short rows' ratios against Go
read too favorable to Xo and small changes against the release before are
hidden. gotchas/wsl-drvfs-exec-cost.md; fixed in f26b951fb.

linux/amd64 micro13 (`xo bench --ab`, 5 rounds, nothing FLAGGED): main
against 0.39.0 is better on map1M 0.904, sort1M 0.645, sort1M_copy 0.912
and spawn100k 0.937, within noise on fib32 1.024, strbuild1M 1.006 and
json10kx20 0.961; against Go, better on all but spawn100k (1.031, within
noise): fib32 0.607, map1M 0.755, strbuild1M 0.573, sort1M 0.861,
sort1M_copy 0.865, json10kx20 0.395.

conc spawn (200 scopes of 1000 tasks, bench/perf/conc/xo/spawn) is the one
confirmed slower row, on darwin/arm64 only (the linux/amd64 spawn100k row is
0.937 of 0.39.0). Its cause is decision 0514 (d654440d2): main's xo building
spawn with main's std (B) against main's std with only 0514's condition
removed from pool.xo worker_main (A), 9 rounds, darwin/arm64: wall 1.24
(1.1 to 1.251) worse, CPU 2.182; A's 21.57 ms matches the 0.39.0 build's
22.17. 0514 measured micro13 spawn100k (one scope of 100,000 tasks), where it
was 0.818 on darwin/arm64; scopes of 1000 tasks lose the chain wake's
fan out and finish slower.
Fixed by decision 0518 (59ce17584, Xo Lang 4): conc spawn built by the
0.39.0 release (A) against b8814fa12's xo with its std plus 59ce17584's
pool.xo change (B), 11 rounds, darwin/arm64: wall 0.973 (0.917 to 1.016),
within noise; CPU 0.925.

coll update (Map.update of 50k keys) is 1.037 (1.019 to 1.048) of 0.39.0 on
darwin/arm64, and code placement: built by each xo at XO_ARM64_PAD 0 to 3,
7 rounds each, main against 0.39.0 is 1.038, 0.985, 1.033 and 0.995; the
pad medians overlap (0.39.0 18.54 to 19.46 ms, main 19.13 to 19.48 ms), so
by pads.py's rule it is placement, not slower.

## The script's report

# Xo against Go: xo 0.39.0 darwin/arm64 (2026-11-05)

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
| llvm | go | 1470 | 30 | 30 | 100 | 316 / 26 / 68 | 2.39 | - |
| coll | go | 1440 | 40 | 40 | 110 | 295 / 25 / 69 | 2.43 | - |
| conc | go | 1430 | 30 | 40 | 100 | 309 / 25 / 68 | 2.40 | - |
| llvm-fib | xo | 1150 | 60 | 40 | 40 | 250 / 40 / 41 | 1.31 | separate, std records |
| llvm-sieve | xo | 1060 | 60 | 40 | 40 | 247 / 40 / 41 | 1.33 | separate, std records |
| llvm-nbody | xo | 1060 | 60 | 40 | 40 | 250 / 41 / 42 | 1.33 | separate, std records |
| llvm-sort | xo | 1050 | 60 | 40 | 40 | 249 / 40 / 42 | 1.33 | separate, std records |
| llvm-interp | xo | 1060 | 60 | 40 | 40 | 250 / 41 / 42 | 1.33 | separate, std records |
| llvm-collatz | xo | 1070 | 60 | 40 | 40 | 250 / 41 / 41 | 1.33 | separate, std records |
| llvm-strings | xo | 2029 | 60 | 40 | 40 | 251 / 40 / 42 | 1.33 | separate, std records |
| coll | xo | 1110 | 130 | 100 | 110 | 254 / 52 / 52 | 1.50 | separate, std records |
| conc-spawn | xo | 1240 | 60 | 40 | 40 | 255 / 40 / 42 | 1.41 | separate, std records |
| conc-fanout | xo | 1220 | 60 | 40 | 40 | 258 / 41 / 42 | 1.41 | separate, std records |
| conc-pingpong | xo | 1210 | 60 | 40 | 40 | 256 / 48 / 48 | 1.43 | separate, std records |
| conc-mutex | xo | 1220 | 60 | 40 | 40 | 255 / 41 / 42 | 1.46 | separate, std records |
| conc-rwmutex | xo | 1230 | 60 | 40 | 40 | 256 / 41 / 43 | 1.47 | separate, std records |
| conc-pmap | xo | 1500 | 70 | 40 | 40 | 257 / 41 / 42 | 1.41 | separate, std records |

The Go column builds one binary per group (llvm, coll, conc); the Xo
programs are one per workload except coll.

## Run (xo bench --ab, A = Go, B = Xo, 3 rounds)

### llvm fib (load 5 at start)

```
A: llvm.go fib 
B: llvm-fib.xo fib 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.66/5.66 5.66/5.66 5.66/5.66

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               235.9       233.1       2.371% 5.969%     0.988   0.954 to 1.104    within noise (n=3)
cpu_ms                233.9       232.1       2.342% 6.223%     0.992   0.953 to 1.102    within noise (n=3)
max_rss_kb            4256        2624        0.752% 0.305%     0.617   0.61 to 0.623     better (n=3)
load: median 5.66 (A) / 5.66 (B); loaded: 0 of 3 rounds (at or above 8.66; baseline 5.66 + 3.0, at least 6.0)
```

### llvm sieve (load 5 at start)

```
A: llvm.go sieve 
B: llvm-sieve.xo sieve 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.66/5.66 5.66/5.66 5.66/5.66

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               113.4       106.6       0.502% 3.564%     0.94    0.887 to 0.963    better (n=3)
cpu_ms                175.7       104.3       6.054% 3.626%     0.593   0.563 to 0.629    better (n=3)
max_rss_kb            47296       13264       2.723% 0%         0.28    0.28 to 0.296     better (n=3)
load: median 5.66 (A) / 5.66 (B); loaded: 0 of 3 rounds (at or above 8.66; baseline 5.66 + 3.0, at least 6.0)
```

### llvm nbody (load 5 at start)

```
A: llvm.go nbody 
B: llvm-nbody.xo nbody 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.66/5.66 5.53/5.53 5.53/5.53

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               165.3       171.5       0.289% 0.459%     1.037   1.037 to 1.041    worse (n=3)
cpu_ms                163.4       169.6       0.087% 0.539%     1.038   1.036 to 1.048    worse (n=3)
max_rss_kb            4160        2624        0.577% 0%         0.631   0.626 to 0.633    better (n=3)
load: median 5.53 (A) / 5.53 (B); loaded: 0 of 3 rounds (at or above 8.66; baseline 5.66 + 3.0, at least 6.0)
```

### llvm sort (load 5 at start)

```
A: llvm.go sort 
B: llvm-sort.xo sort 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.53/5.53 5.53/5.53 5.53/5.53

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               271.4       222.2       0.895% 0.088%     0.819   0.813 to 0.826    better (n=3)
cpu_ms                287.5       221.1       1.335% 0.191%     0.769   0.758 to 0.781    better (n=3)
max_rss_kb            92048       26768       20.99% 0%         0.291   0.264 to 0.427    better (n=3)
load: median 5.53 (A) / 5.53 (B); loaded: 0 of 3 rounds (at or above 8.53; baseline 5.53 + 3.0, at least 6.0)
```

### llvm interp (load 5 at start)

```
A: llvm.go interp 
B: llvm-interp.xo interp 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.53/5.53 5.53/5.53 5.53/5.53

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               147.5       43.66       1.025% 9.153%     0.296   0.292 to 0.349    better (n=3)
cpu_ms                146.6       42.64       1.178% 7.907%     0.291   0.287 to 0.336    better (n=3)
max_rss_kb            4224        2688        2.273% 0.595%     0.636   0.611 to 0.636    better (n=3)
load: median 5.53 (A) / 5.53 (B); loaded: 0 of 3 rounds (at or above 8.53; baseline 5.53 + 3.0, at least 6.0)
```

### llvm collatz (load 5 at start)

```
A: llvm.go collatz 
B: llvm-collatz.xo collatz 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/5.49 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               500.9       459.4       0.566% 0.984%     0.917   0.902 to 0.919    better (n=3)
cpu_ms                500.4       458.5       0.16% 1.134%      0.916   0.898 to 0.919    better (n=3)
max_rss_kb            4240        2624        1.321% 0.915%     0.619   0.619 to 0.636    better (n=3)
load: median 5.49 (A) / 5.49 (B); loaded: 0 of 3 rounds (at or above 8.53; baseline 5.53 + 3.0, at least 6.0)
```

### llvm strings (load 5 at start)

```
A: llvm.go strings 
B: llvm-strings.xo strings 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/5.49 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               93.58       45.37       0.109% 4.219%     0.485   0.48 to 0.521     better (n=3)
cpu_ms                131.8       44.3        0.692% 4.277%     0.336   0.33 to 0.362     better (n=3)
max_rss_kb            27392       18304       5.345% 0%         0.668   0.617 to 0.684    better (n=3)
load: median 5.49 (A) / 5.49 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### coll putget (load 5 at start)

```
A: coll.go putget 
B: coll.xo putget 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/5.49 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               51.08       34.28       0.87% 0.611%      0.671   0.664 to 0.678    better (n=3)
cpu_ms                50.45       33.41       1.265% 0.509%     0.662   0.652 to 0.669    too coarse (n=3)
max_rss_kb            8832        5952        0.815% 0%         0.674   0.669 to 0.68     better (n=3)
load: median 5.49 (A) / 5.49 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### coll get (load 5 at start)

```
A: coll.go get 
B: coll.xo get 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/5.49 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               23.41       14.03       3.216% 4.858%     0.599   0.574 to 0.664    better (n=3)
cpu_ms                22.6        13.06       1.23% 0.299%      0.578   0.573 to 0.588    too coarse (n=3)
max_rss_kb            7088        5952        0.564% 0.403%     0.84    0.84 to 0.847     better (n=3)
load: median 5.49 (A) / 5.49 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### coll push (load 5 at start)

```
A: coll.go push 
B: coll.xo push 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/5.49 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               13.18       9.056       5.452% 0.679%     0.687   0.64 to 0.717     better (n=3)
cpu_ms                18.0        8.209       0.483% 0.713%     0.456   0.45 to 0.46      too coarse (n=3)
max_rss_kb            57968       26960       0.028% 0%         0.465   0.465 to 0.465    better (n=3)
load: median 5.49 (A) / 5.49 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### coll update (load 5 at start)

```
A: coll.go update 
B: coll.xo update 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.53/5.49 5.53/5.53 5.53/5.53

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               30.92       19.56       0.595% 0.458%     0.633   0.625 to 0.638    better (n=3)
cpu_ms                30.39       18.71       0.928% 0.492%     0.616   0.603 to 0.62     too coarse (n=3)
max_rss_kb            8864        5968        3.61% 0%          0.673   0.638 to 0.684    better (n=3)
load: median 5.53 (A) / 5.53 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### coll iter (load 5 at start)

```
A: coll.go iter 
B: coll.xo iter 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.53/5.53 5.53/5.53 5.53/5.53

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               24.69       6.55        3.146% 1.687%     0.265   0.264 to 0.281    better (n=3)
cpu_ms                22.74       5.75        0.508% 1.922%     0.253   0.249 to 0.257    too coarse (n=3)
max_rss_kb            7040        5952        0.341% 0.134%     0.845   0.844 to 0.847    better (n=3)
load: median 5.53 (A) / 5.53 (B); loaded: 0 of 3 rounds (at or above 8.53; baseline 5.53 + 3.0, at least 6.0)
```

### coll copymut (load 5 at start)

```
A: coll.go copymut 
B: coll.xo copymut 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.53/5.53 5.53/5.53 5.53/5.53

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               15.62       10.45       6.298% 0.751%     0.669   0.617 to 0.707    better (n=3)
cpu_ms                15.91       9.706       1.791% 2.261%     0.61    0.582 to 0.632    too coarse (n=3)
max_rss_kb            10432       5984        1.304% 0.134%     0.574   0.574 to 0.588    better (n=3)
load: median 5.53 (A) / 5.53 (B); loaded: 0 of 3 rounds (at or above 8.53; baseline 5.53 + 3.0, at least 6.0)
```

### coll branch (load 5 at start)

```
A: coll.go branch 
B: coll.xo branch 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.53/5.53 5.53/5.53 5.53/5.53

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               215.1       5.321       2.37% 1.889%      0.025   0.025 to 0.026    better (n=3)
cpu_ms                343.6       4.463       2.436% 1.994%     0.013   0.013 to 0.013    better (n=3)
max_rss_kb            15120       5952        10.58% 0.403%     0.394   0.337 to 0.411    better (n=3)
load: median 5.53 (A) / 5.53 (B); loaded: 0 of 3 rounds (at or above 8.53; baseline 5.53 + 3.0, at least 6.0)
```

### conc spawn (load 5 at start)

```
A: conc.go spawn 
B: conc-spawn.xo spawn 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.53/5.53 5.53/5.53 5.53/5.53

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               58.72       27.18       7.18% 9.544%      0.463   0.372 to 0.535    better (n=3)
cpu_ms                196.8       45.5        8.265% 35.37%     0.231   0.105 to 0.313    better (n=3)
max_rss_kb            10960       4224        1.168% 0.947%     0.385   0.378 to 0.395    better (n=3)
load: median 5.53 (A) / 5.53 (B); loaded: 0 of 3 rounds (at or above 8.53; baseline 5.53 + 3.0, at least 6.0)
```

### conc fanout (load 5 at start)

```
A: conc.go fanout 
B: conc-fanout.xo fanout 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.53/5.53 5.53/5.53 5.53/5.53

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               29.04       15.99       18.59% 2.201%     0.551   0.462 to 0.704    better (n=3)
cpu_ms                82.44       15.35       16.73% 1.753%     0.186   0.175 to 0.262    too coarse (n=3)
max_rss_kb            11456       4448        8.17% 0.36%       0.388   0.34 to 0.394     better (n=3)
load: median 5.53 (A) / 5.53 (B); loaded: 0 of 3 rounds (at or above 8.53; baseline 5.53 + 3.0, at least 6.0)
```

### conc pingpong (load 5 at start)

```
A: conc.go pingpong 
B: conc-pingpong.xo pingpong 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.53/5.53 5.53/5.53 5.53/5.53

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               19.62       19.6        1.052% 5.266%     0.999   0.906 to 1.026    within noise (n=3)
cpu_ms                26.25       17.44       2.049% 0.9%       0.664   0.64 to 0.679     too coarse (n=3)
max_rss_kb            4240        2720        2.075% 0%         0.642   0.639 to 0.667    better (n=3)
load: median 5.53 (A) / 5.53 (B); loaded: 0 of 3 rounds (at or above 8.53; baseline 5.53 + 3.0, at least 6.0)
```

### conc mutex (load 5 at start)

```
A: conc.go mutex 
B: conc-mutex.xo mutex 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.53/5.53 5.53/5.53 5.53/5.53

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               23.17       7.391       2.106% 10.49%     0.319   0.3 to 0.378      better (n=3)
cpu_ms                96.14       6.619       1.577% 2.697%     0.069   0.068 to 0.07     too coarse (n=3)
max_rss_kb            4752        2672        1.515% 0%         0.562   0.548 to 0.564    better (n=3)
load: median 5.53 (A) / 5.53 (B); loaded: 0 of 3 rounds (at or above 8.53; baseline 5.53 + 3.0, at least 6.0)
```

### conc rwmutex (load 5 at start)

```
A: conc.go rwmutex 
B: conc-rwmutex.xo rwmutex 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.53/5.53 5.53/5.53 5.53/5.53

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               9.518       9.339       7.092% 2.704%     0.981   0.863 to 1.026    within noise (n=3)
cpu_ms                14.39       8.473       2.259% 1.493%     0.589   0.576 to 0.618    too coarse (n=3)
max_rss_kb            4816        2704        1.163% 0%         0.561   0.561 to 0.575    better (n=3)
load: median 5.53 (A) / 5.53 (B); loaded: 0 of 3 rounds (at or above 8.53; baseline 5.53 + 3.0, at least 6.0)
```

### conc pmap (load 5 at start)

```
A: conc.go pmap 
B: conc-pmap.xo pmap 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.53/5.53 5.53/5.53 5.53/5.53

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               54.77       53.86       0.433% 0.88%      0.983   0.973 to 0.987    better (n=3)
cpu_ms                557.6       552.6       0.131% 0.147%     0.991   0.989 to 0.993    better (n=3)
max_rss_kb            6864        5488        0% 0.292%         0.8     0.795 to 0.8      better (n=3)
load: median 5.53 (A) / 5.53 (B); loaded: 0 of 3 rounds (at or above 8.53; baseline 5.53 + 3.0, at least 6.0)
```


## Test (xo bench --ab, A = go test, B = xo test --std-module, 3 rounds)

Cold empties every cache before each round (Go with -count=1); edit appends one
function to the module under test before each round (Go -count=1). Copies of GOROOT
and the release's std.

### base64 (encoding/base64) cold (load 5 at start)

```
A: goroot/bin/go test -count=1 encoding/base64 
B: ~/xo-wt/l2-rvg040/bin/xo test --std-module=native/base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 8.16/8.16 7.75/7.75 7.37/7.37

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               2188        1464        0.222% 0.993%     0.669   0.663 to 0.677    better (n=3)
cpu_ms                11253       2143        0.816% 1.156%     0.19    0.19 to 0.194     better (n=3)
max_rss_kb            311312      359344      3.482% 0.042%     1.154   1.128 to 1.209    worse (n=3)
load: median 7.75 (A) / 7.75 (B); loaded: 0 of 3 rounds (at or above 8.56; baseline 5.56 + 3.0, at least 6.0)
```

### base64 (encoding/base64) edit (load 5 at start)

```
A: goroot/bin/go test -count=1 encoding/base64 
B: ~/xo-wt/l2-rvg040/bin/xo test --std-module=native/base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.37/5.37 5.37/5.37 5.37/5.37

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               526.1       415.1       3.529% 1.322%     0.789   0.752 to 0.803    better (n=3)
cpu_ms                965.9       406.8       3.342% 3.899%     0.421   0.396 to 0.441    better (n=3)
max_rss_kb            105008      209088      0.663% 0.184%     1.991   1.986 to 2.005    worse (n=3)
load: median 5.37 (A) / 5.37 (B); loaded: 0 of 3 rounds (at or above 8.37; baseline 5.37 + 3.0, at least 6.0)
```

### json (encoding/json) cold (load 5 at start)

```
A: goroot/bin/go test -count=1 encoding/json 
B: ~/xo-wt/l2-rvg040/bin/xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 6.85/6.85 8.38/11.23 10.58/11.23

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               4770        1750        1.894% 6.367%     0.367   0.354 to 0.408    better (n=3)
cpu_ms                24528       2820        0.441% 4.108%     0.115   0.113 to 0.123    better (n=3)
max_rss_kb            317376      378528      4.051% 0.008%     1.193   1.116 to 1.207    worse (n=3)
load: median 8.38 (A) / 11.23 (B); loaded: 2 of 3 rounds (at or above 8.37; baseline 5.37 + 3.0, at least 6.0)
warning: the machine was busy through the run (median load at or above 8.37): wall and cpu ratios are not reliable; measure with HEAVY_CLASS=measure (agent-tools.md Measuring) or --max-load
```

### json (encoding/json) edit (load 5 at start)

```
A: goroot/bin/go test -count=1 encoding/json 
B: ~/xo-wt/l2-rvg040/bin/xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.69/5.69 5.69/5.69 5.69/5.69

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               1062        359.4       1.165% 0.897%     0.338   0.335 to 0.344    better (n=3)
cpu_ms                2485        382.8       1.668% 1.72%      0.154   0.149 to 0.155    better (n=3)
max_rss_kb            162720      226480      0.143% 1.529%     1.392   1.374 to 1.414    worse (n=3)
load: median 5.69 (A) / 5.69 (B); loaded: 0 of 3 rounds (at or above 8.93; baseline 5.93 + 3.0, at least 6.0)
```


## HTTP server (bench/perf/jitter, linux/arm64 offload host)

- host load 0.37 1.69 2.62 1/319 3602176
- release-vs-go-http rvg99632: b8814fa12 on Linux aarch64, 12 CPUs, go1.26.1
- xo xo 0.39.0 linux/arm64; base xo 0.39.0 linux/arm64
- loadgen flags: -c C -rate R -d 10s -warmup 2s -awake (default on in open loop); servers on CPUs 0-5, loadgen on 6-11
- Go's /alloc rows are its /alloc-hand (json.Marshal written by hand, as Xo's derive encodes).
- Built at each XO_ARM64_PAD of HTTP_PADS: the table pools every pad's rounds; the pad sweep verdict below judges xo against base.
- Rounds: 12 per row; each cell is the median, its range in brackets; CPU is the server's CPU microseconds per request.

| row | server | req/s | p50 us | p99 us | p99.9 us | max us | CPU us/req | errors |
|---|---|---|---|---|---|---|---|---|
| /alloc c32 | xo | 168,165 [164,790 to 169,662] | 174 [172 to 177] | 457 [450 to 464] | 1,274 [1,224 to 1,392] | 6,256 [6,102 to 10,968] | 34.3 [33.9 to 35.4] | 0 [0 to 0] |
| /alloc c32 | base | 167,889 [165,750 to 170,064] | 175 [173 to 177] | 456 [451 to 470] | 1,246 [1,216 to 1,354] | 6,354 [6,208 to 10,588] | 34.4 [33.8 to 34.8] | 0 [0 to 0] |
| /alloc c32 | go | 28,109 [28,008 to 28,170] | 484 [482 to 486] | 5,507 [5,491 to 5,535] | 8,211 [8,042 to 8,248] | 15,477 [13,169 to 17,217] | 118.1 [117.5 to 118.5] | 0 [0 to 0] |
| /alloc c64 open 10000/s | xo | 10,000 [10,000 to 10,000] | 78 [78 to 80] | 114 [109 to 122] | 142 [135 to 159] | 226 [201 to 1,443] | 52.0 [51.5 to 53.0] | 0 [0 to 0] |
| /alloc c64 open 10000/s | base | 10,000 [10,000 to 10,000] | 78 [78 to 80] | 114 [110 to 124] | 144 [135 to 165] | 278 [193 to 1,510] | 52.4 [51.5 to 53.3] | 0 [0 to 0] |
| /alloc c64 open 10000/s | go | 10,000 [10,000 to 10,000] | 100 [100 to 102] | 391 [388 to 392] | 503 [498 to 507] | 807 [807 to 1,010] | 122.8 [122.5 to 123.1] | 0 [0 to 0] |
| /alloc c64 open 20000/s | xo | 20,000 [20,000 to 20,000] | 91 [91 to 92] | 144 [142 to 145] | 168 [165 to 172] | 238 [206 to 2,617] | 57.7 [57.1 to 58.4] | 0 [0 to 0] |
| /alloc c64 open 20000/s | base | 20,000 [20,000 to 20,000] | 91 [91 to 92] | 144 [141 to 145] | 168 [166 to 169] | 305 [202 to 2,554] | 57.7 [56.8 to 58.2] | 0 [0 to 0] |
| /alloc c64 open 20000/s | go | 20,000 [20,000 to 20,000] | 190 [188 to 193] | 678 [654 to 678] | 951 [907 to 992] | 2,516 [2,333 to 3,068] | 128.2 [127.6 to 128.6] | 0 [0 to 0] |
| /alloc c256 | xo | 178,750 [173,548 to 181,117] | 1,246 [1,234 to 1,280] | 5,210 [5,116 to 5,327] | 6,484 [6,403 to 6,951] | 10,702 [10,150 to 17,103] | 33.5 [33.1 to 34.5] | 0 [0 to 0] |
| /alloc c256 | base | 178,941 [176,719 to 181,542] | 1,246 [1,228 to 1,258] | 5,166 [5,127 to 5,230] | 6,488 [6,415 to 6,722] | 10,944 [9,937 to 19,761] | 33.5 [33.0 to 33.8] | 0 [0 to 0] |
| /alloc c256 | go | 57,104 [56,850 to 60,780] | 392 [335 to 392] | 38,239 [36,312 to 38,505] | 57,045 [54,105 to 57,689] | 116,178 [107,214 to 122,737] | 81.9 [76.3 to 82.2] | 0 [0 to 0] |
| /hello c32 | xo | 354,106 [347,683 to 355,293] | 87 [86 to 88] | 221 [220 to 228] | 323 [315 to 341] | 4,028 [3,463 to 10,192] | 7.4 [7.3 to 7.4] | 0 [0 to 0] |
| /hello c32 | base | 353,294 [351,363 to 356,302] | 87 [86 to 88] | 221 [221 to 223] | 324 [315 to 332] | 3,722 [3,550 to 10,284] | 7.4 [7.4 to 7.4] | 0 [0 to 0] |
| /hello c32 | go | 275,809 [273,184 to 277,182] | 103 [102 to 104] | 625 [619 to 627] | 837 [823 to 842] | 3,743 [3,426 to 10,269] | 12.3 [12.3 to 12.3] | 0 [0 to 0] |
| /hello c256 | xo | 881,526 [879,125 to 887,106] | 260 [259 to 261] | 832 [817 to 840] | 1,688 [1,625 to 1,726] | 7,531 [6,799 to 9,261] | 5.9 [5.9 to 6.0] | 0 [0 to 0] |
| /hello c256 | base | 882,226 [877,711 to 890,219] | 259 [258 to 261] | 832 [809 to 841] | 1,706 [1,636 to 1,757] | 7,530 [6,815 to 10,660] | 5.9 [5.9 to 6.0] | 0 [0 to 0] |
| /hello c256 | go | 490,355 [487,657 to 492,316] | 305 [305 to 310] | 2,618 [2,607 to 2,637] | 4,229 [4,089 to 4,745] | 10,180 [8,100 to 11,004] | 9.9 [9.8 to 10.0] | 0 [0 to 0] |

- /alloc c32, xo/go medians: req/s 5.983, p50 us 0.360, p99 us 0.083, p99.9 us 0.155, max us 0.404, CPU us/req 0.291
- /alloc c32, xo/base medians: req/s 1.002, p50 us 0.994, p99 us 1.003, p99.9 us 1.023, max us 0.985, CPU us/req 0.999
- /alloc c64 open 10000/s, xo/go medians: req/s 1.000, p50 us 0.780, p99 us 0.292, p99.9 us 0.283, max us 0.279, CPU us/req 0.423
- /alloc c64 open 10000/s, xo/base medians: req/s 1.000, p50 us 0.994, p99 us 1.000, p99.9 us 0.986, max us 0.813, CPU us/req 0.991
- /alloc c64 open 20000/s, xo/go medians: req/s 1.000, p50 us 0.479, p99 us 0.212, p99.9 us 0.177, max us 0.095, CPU us/req 0.450
- /alloc c64 open 20000/s, xo/base medians: req/s 1.000, p50 us 1.000, p99 us 1.003, p99.9 us 1.000, max us 0.780, CPU us/req 1.000
- /alloc c256, xo/go medians: req/s 3.130, p50 us 3.177, p99 us 0.136, p99.9 us 0.114, max us 0.092, CPU us/req 0.410
- /alloc c256, xo/base medians: req/s 0.999, p50 us 1.000, p99 us 1.009, p99.9 us 0.999, max us 0.978, CPU us/req 1.001
- /hello c32, xo/go medians: req/s 1.284, p50 us 0.845, p99 us 0.354, p99.9 us 0.386, max us 1.076, CPU us/req 0.599
- /hello c32, xo/base medians: req/s 1.002, p50 us 1.000, p99 us 1.000, p99.9 us 0.997, max us 1.082, CPU us/req 0.999
- /hello c256, xo/go medians: req/s 1.798, p50 us 0.852, p99 us 0.318, p99.9 us 0.399, max us 0.740, CPU us/req 0.602
- /hello c256, xo/base medians: req/s 0.999, p50 us 1.004, p99 us 0.999, p99.9 us 0.989, max us 1.000, CPU us/req 1.001

pad sweep verdict (reference base): each cell is the median over pads of each pad's median over rounds, the pad medians' range in brackets; slower or faster only when the ranges do not overlap

| row | group | pads | req/s | CPU us/req | req/s vs ref | verdict |
|---|---|---|---|---|---|---|
| /alloc c32 | base | 4 | 168,246 [167,115 to 169,570] | 34.36 [34.08 to 34.59] |  |  |
| /alloc c32 | go | 1 | 28,109 [28,109 to 28,109] | 118.14 [118.14 to 118.14] | 0.167 | one pad: not judged |
| /alloc c32 | xo | 4 | 168,227 [165,328 to 169,544] | 34.35 [34.00 to 35.27] | 1.000 | placement |
| /alloc c64 open 10000/s | base | 4 | 10,000 [10,000 to 10,000] | 52.35 [51.90 to 52.80] |  |  |
| /alloc c64 open 10000/s | go | 1 | 10,000 [10,000 to 10,000] | 122.80 [122.80 to 122.80] | 1.000 | one pad: not judged |
| /alloc c64 open 10000/s | xo | 4 | 10,000 [10,000 to 10,000] | 51.90 [51.60 to 52.90] | 1.000 | placement |
| /alloc c64 open 20000/s | base | 4 | 20,000 [20,000 to 20,000] | 57.73 [57.10 to 58.00] |  |  |
| /alloc c64 open 20000/s | go | 1 | 20,000 [20,000 to 20,000] | 128.20 [128.20 to 128.20] | 1.000 | one pad: not judged |
| /alloc c64 open 20000/s | xo | 4 | 20,000 [20,000 to 20,000] | 57.62 [57.30 to 58.25] | 1.000 | placement |
| /alloc c256 | base | 4 | 178,936 [178,339 to 181,427] | 33.49 [33.02 to 33.61] |  |  |
| /alloc c256 | go | 1 | 57,104 [57,104 to 57,104] | 81.89 [81.89 to 81.89] | 0.319 | one pad: not judged |
| /alloc c256 | xo | 4 | 178,824 [175,237 to 180,459] | 33.52 [33.22 to 34.22] | 0.999 | placement |
| /hello c32 | base | 4 | 353,342 [352,412 to 354,376] | 7.38 [7.38 to 7.40] |  |  |
| /hello c32 | go | 1 | 275,809 [275,809 to 275,809] | 12.30 [12.30 to 12.30] | 0.781 | one pad: not judged |
| /hello c32 | xo | 4 | 354,106 [353,207 to 354,543] | 7.36 [7.32 to 7.40] | 1.002 | placement |
| /hello c256 | base | 4 | 884,167 [878,724 to 887,093] | 5.95 [5.93 to 5.97] |  |  |
| /hello c256 | go | 1 | 490,355 [490,355 to 490,355] | 9.89 [9.89 to 9.89] | 0.555 | one pad: not judged |
| /hello c256 | xo | 4 | 881,557 [880,901 to 885,016] | 5.95 [5.94 to 5.95] | 0.997 | placement |

## linux/amd64 micro set (bench/perf/micro13, the linux/amd64 offload host)

- base: xo 0.39.0 darwin/arm64; new: xo 0.39.0 darwin/arm64; Go go1.26.1; all built for linux/amd64 (xo with --release, as the owner's tests/13-perf)
- each block is xo bench --ab, A = base or go, B = new; ratio is new over A; a row more than 3% slower than base is FLAGGED

```
host Linux x86_64, 28 CPUs, load 0.00
### fib32: base against new
A: rvg-micro13.base fib32 
B: rvg-micro13.new fib32 
5 of 5 rounds used (0 loaded), loads A/B per round: 0/0 0/0 0/0 0/0 0/0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               13.26       13.57       2.66% 4.478%      1.024   0.955 to 1.063    within noise (n=5)
cpu_ms                7.229       5.802       20.09% 15.15%     0.803   0.767 to 1.265    too coarse (n=5)
max_rss_kb            992.0       992.0       0% 0.403%         1.0     0.98 to 1.004     within noise (n=5)
load: median 0 (A) / 0 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0 + 3.0, at least 6.0)
### fib32: go against new
A: rvg-micro13.go fib32 
B: rvg-micro13.new fib32 
5 of 5 rounds used (0 loaded), loads A/B per round: 0/0 0/0 0/0 0/0 0/0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.61       13.71       6.782% 0.404%     0.607   0.57 to 0.632     better (n=5)
cpu_ms                12.81       6.841       9.359% 32.26%     0.534   0.414 to 0.673    too coarse (n=5)
max_rss_kb            2388        988.0       0% 0.405%         0.414   0.414 to 0.415    better (n=5)
load: median 0 (A) / 0 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0 + 3.0, at least 6.0)
### map1M: base against new
A: rvg-micro13.base map1M 
B: rvg-micro13.new map1M 
5 of 5 rounds used (0 loaded), loads A/B per round: 0/0 0/0 0/0 0/0 0/0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               99.19       89.69       9.646% 12.04%     0.904   0.803 to 0.935    better (n=5)
cpu_ms                92.92       81.9        9.231% 8.434%     0.881   0.766 to 0.922    too coarse (n=5)
max_rss_kb            73476       73472       0.305% 0%         1.0     0.997 to 1.003    within noise (n=5)
load: median 0 (A) / 0 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0 + 3.0, at least 6.0)
### map1M: go against new
A: rvg-micro13.go map1M 
B: rvg-micro13.new map1M 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               109.3       82.49       0.786% 1.18%      0.755   0.734 to 0.78     better (n=5)
cpu_ms                108.2       75.52       1.909% 2.377%     0.698   0.673 to 0.752    better (n=5)
max_rss_kb            51892       73472       0.871% 0%         1.416   1.404 to 1.422    worse (n=5)
load: median 0.08 (A) / 0.08 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.08 + 3.0, at least 6.0)
### strbuild1M: base against new
A: rvg-micro13.base strbuild1M 
B: rvg-micro13.new strbuild1M 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               17.98       18.09       0.924% 3.228%     1.006   0.88 to 1.092     within noise (n=5)
cpu_ms                10.63       10.59       22.6% 8.646%      0.996   0.815 to 1.359    too coarse (n=5)
max_rss_kb            12324       12320       0% 0%             1.0     0.981 to 1.018    within noise (n=5)
load: median 0.08 (A) / 0.08 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.08 + 3.0, at least 6.0)
### strbuild1M: go against new
A: rvg-micro13.go strbuild1M 
B: rvg-micro13.new strbuild1M 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               32.74       18.75       21.25% 2.854%     0.573   0.456 to 0.616    better (n=5)
cpu_ms                40.68       11.24       8.066% 16.11%     0.276   0.26 to 0.317     too coarse (n=5)
max_rss_kb            29200       12320       7.0% 1.818%       0.422   0.362 to 0.466    better (n=5)
load: median 0.08 (A) / 0.08 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.08 + 3.0, at least 6.0)
### sort1M: base against new
A: rvg-micro13.base sort1M 
B: rvg-micro13.new sort1M 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               106.6       68.7        0.404% 0.816%     0.645   0.618 to 0.674    better (n=5)
cpu_ms                101.4       61.68       1.657% 1.545%     0.609   0.583 to 0.636    better (n=5)
max_rss_kb            17924       8288        1.25% 2.703%      0.462   0.45 to 0.468     better (n=5)
load: median 0.08 (A) / 0.08 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.08 + 3.0, at least 6.0)
### sort1M: go against new
A: rvg-micro13.go sort1M 
B: rvg-micro13.new sort1M 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               81.16       69.84       0.709% 2.883%     0.861   0.848 to 0.98     better (n=5)
cpu_ms                73.01       62.47       1.085% 0.866%     0.856   0.842 to 0.933    too coarse (n=5)
max_rss_kb            10452       8288        0% 0%             0.793   0.772 to 0.83     better (n=5)
load: median 0.08 (A) / 0.08 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.08 + 3.0, at least 6.0)
### sort1M_copy: base against new
A: rvg-micro13.base sort1M_copy 
B: rvg-micro13.new sort1M_copy 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               75.02       68.45       1.425% 0.513%     0.912   0.906 to 0.922    better (n=5)
cpu_ms                67.44       62.66       0.182% 3.463%     0.929   0.877 to 0.949    too coarse (n=5)
max_rss_kb            18552       8288        0% 0%             0.447   0.44 to 0.447     better (n=5)
load: median 0.08 (A) / 0.08 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.08 + 3.0, at least 6.0)
### sort1M_copy: go against new
A: rvg-micro13.go sort1M_copy 
B: rvg-micro13.new sort1M_copy 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.08/0.08 0.15/0.15 0.15/0.15 0.15/0.15 0.15/0.15

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               79.4        68.69       0.697% 0.609%     0.865   0.838 to 0.963    better (n=5)
cpu_ms                71.51       62.11       0.759% 2.125%     0.869   0.837 to 0.892    too coarse (n=5)
max_rss_kb            10452       8288        2.143% 0%         0.793   0.772 to 0.81     better (n=5)
load: median 0.15 (A) / 0.15 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.08 + 3.0, at least 6.0)
### json10kx20: base against new
A: rvg-micro13.base json10kx20 
B: rvg-micro13.new json10kx20 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.15/0.15 0.15/0.15 0.15/0.15 0.15/0.15 0.15/0.15

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               79.76       76.68       8.187% 3.14%      0.961   0.834 to 1.008    within noise (n=5)
cpu_ms                72.87       70.22       9.518% 1.804%     0.964   0.821 to 0.994    too coarse (n=5)
max_rss_kb            42956       45100       0.531% 0%         1.05    1.039 to 1.052    worse (n=5)
load: median 0.15 (A) / 0.15 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.15 + 3.0, at least 6.0)
### json10kx20: go against new
A: rvg-micro13.go json10kx20 
B: rvg-micro13.new json10kx20 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.15/0.15 0.15/0.15 0.15/0.15 0.15/0.15 0.15/0.15

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               196.1       77.48       1.989% 2.777%     0.395   0.375 to 0.411    better (n=5)
cpu_ms                241.6       70.43       4.122% 3.125%     0.292   0.261 to 0.304    better (n=5)
max_rss_kb            13076       45100       11.17% 0%         3.449   2.895 to 3.507    worse (n=5)
load: median 0.15 (A) / 0.15 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.15 + 3.0, at least 6.0)
### spawn100k: base against new
A: rvg-micro13.base spawn100k 
B: rvg-micro13.new spawn100k 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.15/0.15 0.15/0.15 0.15/0.15 0.15/0.15 0.15/0.15

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               17.54       16.44       3.353% 3.234%     0.937   0.851 to 0.949    better (n=5)
cpu_ms                64.54       56.82       9.073% 5.576%     0.88    0.79 to 0.945     too coarse (n=5)
max_rss_kb            5548        5284        23.58% 1.211%     0.952   0.554 to 1.354    within noise (n=5)
load: median 0.15 (A) / 0.15 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.15 + 3.0, at least 6.0)
### spawn100k: go against new
A: rvg-micro13.go spawn100k 
B: rvg-micro13.new spawn100k 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.15/0.15 0.15/0.15 0.15/0.15 0.15/0.15 0.15/0.15

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               16.14       16.64       1.366% 6.752%     1.031   0.988 to 1.072    within noise (n=5)
cpu_ms                38.76       58.58       6.228% 7.511%     1.511   1.511 to 1.556    too coarse (n=5)
max_rss_kb            8212        5640        0% 50.28%         0.687   0.609 to 1.043    within noise (n=5)
load: median 0.15 (A) / 0.15 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.15 + 3.0, at least 6.0)
OFFLOAD-RC=0
```
