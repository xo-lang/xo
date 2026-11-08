# Xo against Go: xo 0.38.1 darwin/arm64 (2026-11-04)

The release comparison (toolchain.md "Cutting a release" step 7) on the
released darwin/arm64 archive of v0.38.1 (tag ef30e6d25, signed, not yet
notarized: the notary profile was missing, gotchas/notary-profile-missing.md;
notarization does not change the binaries), scripts/release-vs-go.sh from
the private checkout (its last change aae5c430c) with SECTIONS="compile run test" as HEAVY_CLASS=measure on the
Mac (2026-11-04), 3 rounds per row. v0.38.0 was tagged and never released
(a seed only: it crashed on a fault in a leaf Mutex.with closure on x86-64
Linux and macOS), so this report follows release-vs-go-v0.36.0.md. The HTTP
section ran on the linux/arm64 offload host on 2026-11-04 at 4df176695 and
follows the script's report (its first lines say what differs from the tag).
The comparison against the previous release's report is each run row's B/A
ratio (Xo over Go), old against new, a row worse by more than 3% flagged;
flagged wall rows were then measured directly, 0.36.0's build against
0.38.1's of the same program on the linux/arm64 offload host (ab-host.sh,
10 rounds).

## Against 0.36.0

Wall ratios against Go: 19 of the 20 run rows faster than Go and llvm
nbody slower (1.071; 0.36.0: 1.056); the 4 test rows faster than Go.
Better than 0.36.0's ratio by more than 3%: llvm strings (0.532 to 0.485),
coll push (0.849 to 0.752, 0.36.0's darwin push slowdown undone by the
growth reserve, CHANGELOG v0.38.1), coll update, coll iter, conc spawn,
conc fanout and conc pmap (1.044 to 0.990, now faster than Go).

Flagged wall rows (worse than 0.36.0's ratio by more than 3%), settled by
a direct A/B of the two releases' builds of the same program (v0.36.0
5942fb280 against v0.38.1 ef30e6d25, linux/arm64, up to 10 rounds,
stopping early at two agreeing looks): llvm interp 0.304 to 0.319:
0.996, 0.944 to 1.009, within noise. coll get 0.623 to 0.649: 0.953,
better. coll branch 0.024 to 0.026 (5 ms rows): 0.863, better. conc
pingpong 0.877 to 0.925: 0.958, better. conc mutex 0.271 to 0.284: 0.743,
better. conc rwmutex 0.892 to 0.928: 1.090, 1.008 to 1.103, worse at an
early stop over 7 to 8% spreads; 20 rounds then gave 0.986, 0.97 to 1.013,
within noise. No flagged run row is slower in the direct A/B.

The test rows (cold and edit `xo test --std-module` of base64 and json)
are flagged on wall (base64 cold 0.566 to 0.640, json cold 0.336 to
0.369) while their cpu ratios fell (base64 cold 0.236 to 0.187, json cold
0.130 to 0.105): this run's load was 5 against 4, and xo's cold test
builds run on all cores; they were not measured directly (ab-host times
programs, not `xo test`).

Memory: every run row's peak is lower than Go's (0.353 to 0.864) and
none rose by more than 3% against 0.36.0's ratio except conc fanout
(0.384 to 0.402, 4,544 to 4,912 KB); llvm sort 0.624 to 0.353, coll push
1.032 to 0.459, coll putget 0.982 to 0.714. In the direct A/Bs on
linux/arm64 every program's max RSS fell (0.247 to 0.541 of 0.36.0's).
The test rows' peaks stay above Go's (1.154 to 1.937).

# Xo against Go: xo 0.38.1 darwin/arm64 (2026-11-04)

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
| llvm | go | 1590 | 40 | 30 | 100 | 300 / 26 / 67 | 2.39 | - |
| coll | go | 1540 | 40 | 40 | 110 | 295 / 26 / 68 | 2.43 | - |
| conc | go | 1450 | 40 | 30 | 100 | 296 / 26 / 68 | 2.40 | - |
| llvm-fib | xo | 1160 | 60 | 30 | 30 | 255 / 39 / 41 | 1.29 | separate, std records |
| llvm-sieve | xo | 1050 | 50 | 30 | 30 | 255 / 39 / 41 | 1.29 | separate, std records |
| llvm-nbody | xo | 1070 | 60 | 40 | 40 | 253 / 40 / 41 | 1.29 | separate, std records |
| llvm-sort | xo | 1050 | 60 | 40 | 40 | 255 / 40 / 41 | 1.29 | separate, std records |
| llvm-interp | xo | 1070 | 60 | 40 | 40 | 254 / 40 / 42 | 1.29 | separate, std records |
| llvm-collatz | xo | 1050 | 70 | 30 | 40 | 255 / 39 / 41 | 1.29 | separate, std records |
| llvm-strings | xo | 1080 | 60 | 30 | 40 | 254 / 39 / 41 | 1.29 | separate, std records |
| coll | xo | 1110 | 130 | 100 | 100 | 257 / 49 / 49 | 1.41 | separate, std records |
| conc-spawn | xo | 1230 | 60 | 40 | 40 | 258 / 40 / 41 | 1.38 | separate, std records |
| conc-fanout | xo | 1210 | 60 | 40 | 40 | 258 / 40 / 41 | 1.39 | separate, std records |
| conc-pingpong | xo | 1240 | 60 | 40 | 40 | 258 / 46 / 46 | 1.43 | separate, std records |
| conc-mutex | xo | 1200 | 60 | 40 | 40 | 258 / 40 / 41 | 1.44 | separate, std records |
| conc-rwmutex | xo | 1190 | 60 | 40 | 40 | 257 / 40 / 42 | 1.44 | separate, std records |
| conc-pmap | xo | 1210 | 60 | 40 | 40 | 258 / 40 / 41 | 1.39 | separate, std records |

The Go column builds one binary per group (llvm, coll, conc); the Xo
programs are one per workload except coll.

## Run (xo bench --ab, A = Go, B = Xo, 3 rounds)

### llvm fib (load 5 at start)

```
A: llvm.go fib 
B: llvm-fib.xo fib 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.77/5.77 5.77/5.77 5.77/5.77

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               239.5       225.6       1.336% 2.391%     0.942   0.911 to 0.967    better (n=3)
cpu_ms                238.7       224.8       1.407% 2.116%     0.942   0.91 to 0.961     better (n=3)
max_rss_kb            4192        2592        0.573% 0%         0.618   0.616 to 0.623    better (n=3)
load: median 5.77 (A) / 5.77 (B); loaded: 0 of 3 rounds (at or above 8.77; baseline 5.77 + 3.0, at least 6.0)
```

### llvm sieve (load 5 at start)

```
A: llvm.go sieve 
B: llvm-sieve.xo sieve 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.77/5.77 5.77/5.77 5.77/5.77

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               113.8       107.7       1.322% 4.694%     0.946   0.924 to 0.987    better (n=3)
cpu_ms                187.8       106.8       4.375% 3.375%     0.569   0.552 to 0.628    better (n=3)
max_rss_kb            46016       17136       3.72% 0%          0.372   0.356 to 0.383    better (n=3)
load: median 5.77 (A) / 5.77 (B); loaded: 0 of 3 rounds (at or above 8.77; baseline 5.77 + 3.0, at least 6.0)
```

### llvm nbody (load 5 at start)

```
A: llvm.go nbody 
B: llvm-nbody.xo nbody 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.77/5.77 5.77/5.77 5.77/5.77

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               163.8       175.5       0.247% 0.491%     1.071   1.062 to 1.072    worse (n=3)
cpu_ms                162.9       174.8       0.264% 0.833%     1.073   1.055 to 1.073    worse (n=3)
max_rss_kb            4160        2592        0.577% 0%         0.623   0.616 to 0.623    better (n=3)
load: median 5.77 (A) / 5.77 (B); loaded: 0 of 3 rounds (at or above 8.77; baseline 5.77 + 3.0, at least 6.0)
```

### llvm sort (load 5 at start)

```
A: llvm.go sort 
B: llvm-sort.xo sort 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.71/5.77 5.71/5.71 5.71/5.71

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               269.6       219.3       0.365% 0.126%     0.813   0.811 to 0.819    better (n=3)
cpu_ms                285.9       218.6       0.5% 0.122%       0.764   0.759 to 0.766    better (n=3)
max_rss_kb            75792       26720       16.84% 0.09%      0.353   0.297 to 0.415    better (n=3)
load: median 5.71 (A) / 5.71 (B); loaded: 0 of 3 rounds (at or above 8.77; baseline 5.77 + 3.0, at least 6.0)
```

### llvm interp (load 5 at start)

```
A: llvm.go interp 
B: llvm-interp.xo interp 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.71/5.71 5.71/5.71 5.71/5.71

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               146.7       46.83       0.992% 8.152%     0.319   0.282 to 0.33     better (n=3)
cpu_ms                146.1       46.1        0.914% 7.943%     0.316   0.279 to 0.326    better (n=3)
max_rss_kb            4240        2592        0.189% 0%         0.611   0.609 to 0.611    better (n=3)
load: median 5.71 (A) / 5.71 (B); loaded: 0 of 3 rounds (at or above 8.71; baseline 5.71 + 3.0, at least 6.0)
```

### llvm collatz (load 5 at start)

```
A: llvm.go collatz 
B: llvm-collatz.xo collatz 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.71/5.71 5.71/5.71 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               500.8       452.2       0.022% 0.512%     0.903   0.899 to 0.908    better (n=3)
cpu_ms                499.9       451.3       0.125% 0.523%     0.903   0.9 to 0.908      better (n=3)
max_rss_kb            4192        2592        1.145% 0%         0.618   0.614 to 0.628    better (n=3)
load: median 5.71 (A) / 5.71 (B); loaded: 0 of 3 rounds (at or above 8.71; baseline 5.71 + 3.0, at least 6.0)
```

### llvm strings (load 5 at start)

```
A: llvm.go strings 
B: llvm-strings.xo strings 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/5.49 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               91.69       44.43       1.756% 4.64%      0.485   0.47 to 0.498     better (n=3)
cpu_ms                128.8       43.41       1.668% 4.997%     0.337   0.331 to 0.355    better (n=3)
max_rss_kb            23200       16128       9.897% 0%         0.695   0.589 to 0.707    better (n=3)
load: median 5.49 (A) / 5.49 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### coll putget (load 5 at start)

```
A: coll.go putget 
B: coll.xo putget 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/5.49 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               50.5        30.31       0.712% 5.685%     0.6     0.576 to 0.645    better (n=3)
cpu_ms                49.9        29.6        0.914% 5.471%     0.593   0.569 to 0.631    too coarse (n=3)
max_rss_kb            8784        6272        0.638% 0%         0.714   0.713 to 0.722    better (n=3)
load: median 5.49 (A) / 5.49 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### coll get (load 5 at start)

```
A: coll.go get 
B: coll.xo get 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/5.49 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.92       14.88       1.366% 1.18%      0.649   0.625 to 0.657    better (n=3)
cpu_ms                22.42       14.07       1.623% 1.169%     0.628   0.61 to 0.646     too coarse (n=3)
max_rss_kb            7040        6080        1.25% 0%          0.864   0.846 to 0.868    better (n=3)
load: median 5.49 (A) / 5.49 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### coll push (load 5 at start)

```
A: coll.go push 
B: coll.xo push 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/5.49 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               12.74       9.572       4.75% 4.994%      0.752   0.645 to 0.783    better (n=3)
cpu_ms                17.45       8.651       6.898% 5.496%     0.496   0.416 to 0.518    too coarse (n=3)
max_rss_kb            58320       26768       10.44% 0.06%      0.459   0.381 to 0.461    better (n=3)
load: median 5.49 (A) / 5.49 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### coll update (load 5 at start)

```
A: coll.go update 
B: coll.xo update 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/5.49 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               31.21       18.88       1.049% 1.997%     0.605   0.6 to 0.638      better (n=3)
cpu_ms                30.67       18.14       0.955% 1.693%     0.591   0.589 to 0.618    too coarse (n=3)
max_rss_kb            8752        6272        0.548% 0.128%     0.717   0.715 to 0.723    better (n=3)
load: median 5.49 (A) / 5.49 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### coll iter (load 5 at start)

```
A: coll.go iter 
B: coll.xo iter 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/5.49 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.8        5.762       1.437% 3.332%     0.253   0.25 to 0.259     better (n=3)
cpu_ms                21.88       5.015       1.467% 1.406%     0.229   0.225 to 0.229    too coarse (n=3)
max_rss_kb            7136        6128        0.448% 0.261%     0.859   0.855 to 0.859    better (n=3)
load: median 5.49 (A) / 5.49 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### coll copymut (load 5 at start)

```
A: coll.go copymut 
B: coll.xo copymut 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/5.49 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               14.24       9.706       1.197% 1.504%     0.681   0.664 to 0.681    better (n=3)
cpu_ms                14.67       8.893       0.852% 1.749%     0.606   0.589 to 0.615    too coarse (n=3)
max_rss_kb            10144       6080        1.972% 0%         0.599   0.591 to 0.615    better (n=3)
load: median 5.49 (A) / 5.49 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### coll branch (load 5 at start)

```
A: coll.go branch 
B: coll.xo branch 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/5.49 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               203.9       5.292       2.111% 0.482%     0.026   0.025 to 0.026    better (n=3)
cpu_ms                324.6       4.442       0.771% 1.148%     0.014   0.014 to 0.014    better (n=3)
max_rss_kb            16848       6080        6.885% 0.132%     0.361   0.323 to 0.367    better (n=3)
load: median 5.49 (A) / 5.49 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### conc spawn (load 5 at start)

```
A: conc.go spawn 
B: conc-spawn.xo spawn 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/5.49 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               51.09       21.75       11.63% 3.016%     0.426   0.368 to 0.429    better (n=3)
cpu_ms                172.0       28.94       12.24% 10.22%     0.168   0.134 to 0.169    better (n=3)
max_rss_kb            10992       5840        2.693% 5.068%     0.531   0.495 to 0.538    better (n=3)
load: median 5.49 (A) / 5.49 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### conc fanout (load 5 at start)

```
A: conc.go fanout 
B: conc-fanout.xo fanout 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/5.49 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               25.12       15.95       6.833% 1.006%     0.635   0.599 to 0.685    better (n=3)
cpu_ms                72.03       15.65       5.729% 4.427%     0.217   0.209 to 0.233    too coarse (n=3)
max_rss_kb            12208       4912        3.932% 11.24%     0.402   0.374 to 0.46     better (n=3)
load: median 5.49 (A) / 5.49 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### conc pingpong (load 5 at start)

```
A: conc.go pingpong 
B: conc-pingpong.xo pingpong 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/5.49 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               19.39       17.93       2.014% 0.505%     0.925   0.914 to 0.942    better (n=3)
cpu_ms                25.67       17.19       1.995% 0.442%     0.67    0.663 to 0.684    too coarse (n=3)
max_rss_kb            4320        2736        2.593% 0%         0.633   0.624 to 0.658    better (n=3)
load: median 5.49 (A) / 5.49 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### conc mutex (load 5 at start)

```
A: conc.go mutex 
B: conc-mutex.xo mutex 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/5.49 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               23.07       6.543       1.166% 2.422%     0.284   0.278 to 0.29     better (n=3)
cpu_ms                96.91       5.845       1.734% 1.36%      0.06    0.06 to 0.062     too coarse (n=3)
max_rss_kb            4816        2704        0.498% 0%         0.561   0.561 to 0.567    better (n=3)
load: median 5.49 (A) / 5.49 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### conc rwmutex (load 5 at start)

```
A: conc.go rwmutex 
B: conc-rwmutex.xo rwmutex 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/5.49 5.49/5.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               9.385       8.706       2.014% 0.477%     0.928   0.9 to 0.937      better (n=3)
cpu_ms                14.71       8.029       3.471% 1.177%     0.546   0.512 to 0.554    too coarse (n=3)
max_rss_kb            4816        2720        1.163% 0%         0.565   0.554 to 0.567    better (n=3)
load: median 5.49 (A) / 5.49 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```

### conc pmap (load 5 at start)

```
A: conc.go pmap 
B: conc-pmap.xo pmap 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.49/5.49 5.49/6.41 6.41/6.41

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               59.64       59.04       0.288% 0.509%     0.99    0.983 to 0.999    better (n=3)
cpu_ms                562.4       558.3       0.068% 0.118%     0.993   0.992 to 0.995    better (n=3)
max_rss_kb            6720        5376        1.071% 0.446%     0.8     0.792 to 0.81     better (n=3)
load: median 5.49 (A) / 6.41 (B); loaded: 0 of 3 rounds (at or above 8.49; baseline 5.49 + 3.0, at least 6.0)
```


## Test (xo bench --ab, A = go test, B = xo test --std-module, 3 rounds)

Cold empties every cache before each round (Go with -count=1); edit appends one
function to the module under test before each round (Go -count=1). Copies of GOROOT
and the release's std.

### base64 (encoding/base64) cold (load 5 at start)

```
A: goroot/bin/go test -count=1 encoding/base64 
B: ~/xo-wt/rvg038-unpacked/xo-v0.38.1-darwin-arm64/bin/xo test --std-module=native/base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 6.06/6.06 5.81/5.81 5.51/5.81

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               2044        1309        0.862% 0.383%     0.64    0.628 to 0.643    better (n=3)
cpu_ms                11414       2130        0.327% 0.345%     0.187   0.185 to 0.188    better (n=3)
max_rss_kb            310512      358352      3.357% 0.185%     1.154   1.123 to 1.204    worse (n=3)
load: median 5.81 (A) / 5.81 (B); loaded: 0 of 3 rounds (at or above 8.37; baseline 5.37 + 3.0, at least 6.0)
```

### base64 (encoding/base64) edit (load 5 at start)

```
A: goroot/bin/go test -count=1 encoding/base64 
B: ~/xo-wt/rvg038-unpacked/xo-v0.38.1-darwin-arm64/bin/xo test --std-module=native/base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.63/5.63 5.63/5.63 5.42/5.42

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               318.9       268.7       1.205% 1.433%     0.843   0.83 to 0.843     better (n=3)
cpu_ms                978.6       394.9       0.944% 2.468%     0.404   0.39 to 0.404     better (n=3)
max_rss_kb            104496      202368      0.299% 1.241%     1.937   1.928 to 1.968    worse (n=3)
load: median 5.63 (A) / 5.63 (B); loaded: 0 of 3 rounds (at or above 8.63; baseline 5.63 + 3.0, at least 6.0)
```

### json (encoding/json) cold (load 5 at start)

```
A: goroot/bin/go test -count=1 encoding/json 
B: ~/xo-wt/rvg038-unpacked/xo-v0.38.1-darwin-arm64/bin/xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.22/5.22 8.57/8.12 8.12/8.12

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               3960        1463        1.53% 0.559%      0.369   0.358 to 0.371    better (n=3)
cpu_ms                23812       2500        1.092% 0.69%      0.105   0.103 to 0.106    better (n=3)
max_rss_kb            307296      379744      0.804% 0.103%     1.236   1.227 to 1.244    worse (n=3)
load: median 8.12 (A) / 8.12 (B); loaded: 1 of 3 rounds (at or above 8.42; baseline 5.42 + 3.0, at least 6.0)
```

### json (encoding/json) edit (load 5 at start)

```
A: goroot/bin/go test -count=1 encoding/json 
B: ~/xo-wt/rvg038-unpacked/xo-v0.38.1-darwin-arm64/bin/xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.22/5.22 5.22/5.22 5.22/5.22

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               828.0       203.5       0.493% 0.323%     0.246   0.246 to 0.247    better (n=3)
cpu_ms                2681        337.5       2.332% 0.336%     0.126   0.122 to 0.127    better (n=3)
max_rss_kb            162848      217200      0.162% 1.532%     1.334   1.317 to 1.355    worse (n=3)
load: median 5.22 (A) / 5.22 (B); loaded: 0 of 3 rounds (at or above 8.22; baseline 5.22 + 3.0, at least 6.0)
```


## HTTP server (bench/perf/jitter, linux/arm64 offload host)

- host load 0.01 0.88 2.06 1/310 2086410
- release-vs-go-http rvg18473: 4df1766956 on Linux aarch64, 12 CPUs, go1.26.1
- Measured at 4df176695 (eda09dfa0 plus the version commit). The 0.38.1 release commit, ef30e6d25, adds only 9ea297c50 and 5b95711e3 (the x86 twin, same scope) and the 0.38.1 version: the arm64 unwind regions and landing pad entries of leaf functions whose fault stubs have landing pads. It changes no code a request runs unless such a fault happens.
- xo xo 0.38.0 linux/arm64; base xo 0.36.0 linux/arm64
- loadgen flags: -c C -rate R -d 10s -warmup 2s -awake (default on in open loop); servers on CPUs 0-5, loadgen on 6-11
- Go's /alloc rows are its /alloc-hand (json.Marshal written by hand, as Xo's derive encodes).
- Built at each XO_ARM64_PAD of HTTP_PADS: the table pools every pad's rounds; the pad sweep verdict below judges xo against base.
- Rounds: 12 per row; each cell is the median, its range in brackets; CPU is the server's CPU microseconds per request.

| row | server | req/s | p50 us | p99 us | p99.9 us | max us | CPU us/req | errors |
|---|---|---|---|---|---|---|---|---|
| /alloc c32 | xo | 169,125 [167,675 to 173,353] | 173 [169 to 175] | 456 [446 to 461] | 1,230 [1,185 to 1,300] | 6,260 [6,137 to 10,622] | 34.1 [33.1 to 34.4] | 0 [0 to 0] |
| /alloc c32 | base | 169,409 [165,531 to 171,240] | 173 [171 to 177] | 452 [448 to 461] | 1,242 [1,190 to 1,365] | 6,244 [6,142 to 6,387] | 34.1 [33.6 to 34.9] | 0 [0 to 0] |
| /alloc c32 | go | 28,152 [27,974 to 28,327] | 482 [472 to 485] | 5,505 [5,480 to 5,528] | 8,122 [7,976 to 8,225] | 13,930 [13,135 to 16,776] | 117.8 [116.3 to 119.0] | 0 [0 to 0] |
| /alloc c64 open 10000/s | xo | 10,000 [10,000 to 10,000] | 78 [78 to 80] | 112 [109 to 122] | 138 [128 to 148] | 214 [179 to 1,261] | 52.1 [51.3 to 53.1] | 0 [0 to 0] |
| /alloc c64 open 10000/s | base | 10,000 [10,000 to 10,000] | 78 [78 to 80] | 113 [111 to 124] | 145 [137 to 155] | 216 [178 to 1,121] | 52.0 [51.3 to 53.1] | 0 [0 to 0] |
| /alloc c64 open 10000/s | go | 10,000 [10,000 to 10,000] | 100 [100 to 101] | 392 [390 to 400] | 509 [504 to 544] | 889 [710 to 2,290] | 123.2 [122.7 to 124.1] | 0 [0 to 0] |
| /alloc c64 open 20000/s | xo | 20,000 [20,000 to 20,000] | 91 [90 to 91] | 143 [142 to 152] | 169 [165 to 185] | 288 [218 to 2,181] | 57.5 [56.6 to 57.8] | 0 [0 to 0] |
| /alloc c64 open 20000/s | base | 20,000 [20,000 to 20,000] | 91 [90 to 97] | 142 [140 to 710] | 167 [165 to 16,026] | 243 [209 to 24,819] | 57.1 [56.5 to 63.6] | 0 [0 to 0] |
| /alloc c64 open 20000/s | go | 20,000 [20,000 to 20,000] | 182 [177 to 188] | 661 [645 to 662] | 923 [884 to 2,170] | 2,702 [2,113 to 12,475] | 126.3 [123.8 to 127.7] | 0 [0 to 0] |
| /alloc c256 | xo | 180,012 [177,747 to 185,713] | 1,240 [1,198 to 1,256] | 5,158 [5,084 to 5,209] | 6,396 [6,289 to 6,550] | 10,256 [10,024 to 10,571] | 33.3 [32.3 to 33.7] | 0 [0 to 0] |
| /alloc c256 | base | 179,899 [173,242 to 184,339] | 1,238 [1,216 to 1,283] | 5,194 [5,013 to 5,306] | 6,513 [6,309 to 6,860] | 10,716 [9,555 to 14,786] | 33.3 [32.5 to 34.6] | 0 [0 to 0] |
| /alloc c256 | go | 57,207 [57,118 to 57,628] | 387 [376 to 387] | 38,422 [38,134 to 38,655] | 57,726 [57,116 to 57,855] | 114,795 [102,632 to 115,725] | 81.8 [81.3 to 82.0] | 0 [0 to 0] |
| /hello c32 | xo | 356,005 [346,088 to 360,674] | 86 [85 to 89] | 221 [218 to 229] | 322 [314 to 331] | 3,804 [3,491 to 9,997] | 7.3 [7.2 to 7.4] | 0 [0 to 0] |
| /hello c32 | base | 356,090 [345,571 to 359,192] | 86 [85 to 89] | 220 [219 to 231] | 320 [316 to 341] | 4,047 [3,525 to 10,179] | 7.3 [7.2 to 7.5] | 0 [0 to 0] |
| /hello c32 | go | 275,769 [270,482 to 278,547] | 102 [101 to 104] | 631 [629 to 631] | 847 [836 to 864] | 3,632 [3,348 to 10,213] | 12.3 [12.2 to 12.6] | 0 [0 to 0] |
| /hello c256 | xo | 891,956 [876,642 to 901,506] | 257 [254 to 260] | 818 [800 to 846] | 1,668 [1,610 to 1,763] | 9,040 [6,155 to 12,018] | 5.9 [5.8 to 5.9] | 0 [0 to 0] |
| /hello c256 | base | 892,745 [882,733 to 897,789] | 256 [255 to 259] | 816 [803 to 827] | 1,680 [1,569 to 1,752] | 8,226 [6,751 to 10,158] | 5.9 [5.8 to 5.9] | 0 [0 to 0] |
| /hello c256 | go | 488,810 [486,020 to 495,595] | 307 [306 to 310] | 2,615 [2,585 to 2,624] | 4,323 [4,267 to 4,496] | 8,753 [7,761 to 10,295] | 10.0 [9.8 to 10.0] | 0 [0 to 0] |

- /alloc c32, xo/go medians: req/s 6.008, p50 us 0.359, p99 us 0.083, p99.9 us 0.151, max us 0.449, CPU us/req 0.289
- /alloc c32, xo/base medians: req/s 0.998, p50 us 1.000, p99 us 1.008, p99.9 us 0.990, max us 1.003, CPU us/req 1.000
- /alloc c64 open 10000/s, xo/go medians: req/s 1.000, p50 us 0.785, p99 us 0.287, p99.9 us 0.272, max us 0.240, CPU us/req 0.423
- /alloc c64 open 10000/s, xo/base medians: req/s 1.000, p50 us 1.006, p99 us 0.996, p99.9 us 0.955, max us 0.991, CPU us/req 1.002
- /alloc c64 open 20000/s, xo/go medians: req/s 1.000, p50 us 0.500, p99 us 0.216, p99.9 us 0.183, max us 0.107, CPU us/req 0.456
- /alloc c64 open 20000/s, xo/base medians: req/s 1.000, p50 us 1.000, p99 us 1.004, p99.9 us 1.012, max us 1.187, CPU us/req 1.008
- /alloc c256, xo/go medians: req/s 3.147, p50 us 3.203, p99 us 0.134, p99.9 us 0.111, max us 0.089, CPU us/req 0.407
- /alloc c256, xo/base medians: req/s 1.001, p50 us 1.002, p99 us 0.993, p99.9 us 0.982, max us 0.957, CPU us/req 0.999
- /hello c32, xo/go medians: req/s 1.291, p50 us 0.843, p99 us 0.350, p99.9 us 0.380, max us 1.047, CPU us/req 0.590
- /hello c32, xo/base medians: req/s 1.000, p50 us 1.000, p99 us 1.005, p99.9 us 1.006, max us 0.940, CPU us/req 1.001
- /hello c256, xo/go medians: req/s 1.825, p50 us 0.837, p99 us 0.313, p99.9 us 0.386, max us 1.033, CPU us/req 0.589
- /hello c256, xo/base medians: req/s 0.999, p50 us 1.002, p99 us 1.002, p99.9 us 0.993, max us 1.099, CPU us/req 0.999

pad sweep verdict (reference base): each cell is the median over pads of each pad's median over rounds, the pad medians' range in brackets; slower or faster only when the ranges do not overlap

| row | group | pads | req/s | CPU us/req | req/s vs ref | verdict |
|---|---|---|---|---|---|---|
| /alloc c32 | base | 4 | 169,409 [166,244 to 171,017] | 34.05 [33.59 to 34.73] |  |  |
| /alloc c32 | go | 1 | 28,152 [28,152 to 28,152] | 117.75 [117.75 to 117.75] | 0.166 | one pad: not judged |
| /alloc c32 | xo | 4 | 169,141 [167,690 to 172,285] | 34.05 [33.21 to 34.34] | 0.998 | placement |
| /alloc c64 open 10000/s | base | 4 | 10,000 [10,000 to 10,000] | 52.20 [51.70 to 52.60] |  |  |
| /alloc c64 open 10000/s | go | 1 | 10,000 [10,000 to 10,000] | 123.20 [123.20 to 123.20] | 1.000 | one pad: not judged |
| /alloc c64 open 10000/s | xo | 4 | 10,000 [10,000 to 10,000] | 52.10 [51.50 to 52.40] | 1.000 | placement |
| /alloc c64 open 20000/s | base | 4 | 20,000 [20,000 to 20,000] | 56.97 [56.55 to 58.00] |  |  |
| /alloc c64 open 20000/s | go | 1 | 20,000 [20,000 to 20,000] | 126.30 [126.30 to 126.30] | 1.000 | one pad: not judged |
| /alloc c64 open 20000/s | xo | 4 | 20,000 [20,000 to 20,000] | 57.52 [56.70 to 57.70] | 1.000 | faster |
| /alloc c256 | base | 4 | 179,899 [174,641 to 183,313] | 33.32 [32.69 to 34.30] |  |  |
| /alloc c256 | go | 1 | 57,207 [57,207 to 57,207] | 81.84 [81.84 to 81.84] | 0.318 | one pad: not judged |
| /alloc c256 | xo | 4 | 179,763 [178,003 to 184,667] | 33.34 [32.46 to 33.66] | 0.999 | placement |
| /hello c32 | base | 4 | 356,017 [353,575 to 358,001] | 7.27 [7.25 to 7.29] |  |  |
| /hello c32 | go | 1 | 275,769 [275,769 to 275,769] | 12.33 [12.33 to 12.33] | 0.775 | one pad: not judged |
| /hello c32 | xo | 4 | 357,033 [354,255 to 358,028] | 7.28 [7.26 to 7.29] | 1.003 | placement |
| /hello c256 | base | 4 | 893,324 [892,372 to 894,700] | 5.87 [5.84 to 5.90] |  |  |
| /hello c256 | go | 1 | 488,810 [488,810 to 488,810] | 9.96 [9.96 to 9.96] | 0.547 | one pad: not judged |
| /hello c256 | xo | 4 | 892,181 [888,708 to 896,388] | 5.87 [5.84 to 5.88] | 0.999 | placement |

## linux/amd64 micro set (added 2026-11-04)

Note (2026-11-08): release-vs-go-amd64.sh ran these binaries from /mnt/c,
where drvfs adds about 6 ms of exec to every run (fib32 13.8 against 7.9 ms
from ext4), more for Go's larger binary, so the short rows' ratios against Go
read too favorable to Xo and small changes against the release before are
hidden. gotchas/wsl-drvfs-exec-cost.md; fixed in f26b951fb.

The section this report did not have (Lead, 2026-11-04: Xo Lang 1 found
json10kx20 1.78 times slower on linux/amd64 than in 0.35.0, unseen here):
scripts/release-vs-go.sh SECTIONS=amd64 with the released darwin/arm64 xo of
v0.38.1 and of v0.36.0 (AMD64_BASE_XO, from the v0.36.0 GitHub release,
sha256 checked), bench/perf/micro13 built for linux/amd64 with --release by
each and by Go 1.26.1, run on the linux/amd64 offload host (WSL2, 28 CPUs,
load 5.4 to 5.8) by scripts/release-vs-go-amd64.sh, 5 rounds of xo bench
--ab per row. Wall medians; the ratios are 0.38.1 over the other, with the
95% interval and the verdict.

| workload | 0.36.0 ms | 0.38.1 ms | 0.38.1 / 0.36.0 | Go ms | 0.38.1 / Go |
|---|---|---|---|---|---|
| fib32 | 13.71 | 13.21 | 0.963 (0.957 to 1.014, within noise (n=5)) | 22.38 | 0.623 (0.585 to 0.63) |
| map1M | 96.99 | 94.59 | 0.975 (0.947 to 0.993, better (n=5)) | 121.8 | 0.771 (0.733 to 0.783) |
| strbuild1M | 21.88 | 17.45 | 0.797 (0.779 to 0.813, better (n=5)) | 33.03 | 0.563 (0.436 to 0.588) |
| sort1M | 80.46 | 79.46 | 0.988 (0.974 to 0.999, better (n=5)) | 82.49 | 0.965 (0.955 to 0.974) |
| json10kx20 | 60.63 | 94.43 | 1.557 (1.549 to 1.683, worse (n=5)) FLAGGED | 192.4 | 0.491 (0.481 to 0.518) |
| spawn100k | 16.82 | 17.11 | 1.017 (0.793 to 1.061, within noise (n=5)) | 17.09 | 1.023 (0.986 to 1.082) |

json10kx20 is the one regression against 0.36.0 (1.557; still 0.49 of Go,
whose encoding/json is slow on this host). Xo Lang 1 owns its bisect and fix.
The section is now a required part of the release report (toolchain.md
"Cutting a release" step 7).
