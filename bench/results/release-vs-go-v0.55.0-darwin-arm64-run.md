# Xo against Go: xo 0.55.0 darwin/arm64 (2026-11-22)

The run section only (task #686), with the REPEAT counts of
scripts/release-vs-go.sh at 98335486d8 (bench/perf's programs as of that
commit) and the published 0.55.0 darwin/arm64 archive, as one heavy.sh
measure job on the Mac Studio (SECTIONS=run; HEAVY_CLASS=measure). The run
section of release-vs-go-v0.55.0.md failed (task #686's darwin fix); this
file supplies it. The Mac was not idle: load about 5 of 30 cores at each
pair's start, 0 rounds loaded by xo bench's rule. 20 of 20 rows have a wall
verdict: 16 better, 2 worse (fib 1.124, rwmutex 4.356), 2 within noise
(sieve, pmap). The build and size paragraphs below are the script's
standard header; this file has no compile rows.

scripts/release-vs-go.sh on Mac Studio (Mac17,15), Apple M5 Ultra, 30 cores, 96 GB, Darwin arm64,
Go go1.26.11. Build times are wall milliseconds of one build; peak MB is the
build's largest resident set (cold / warm / edit). Warm is the third unchanged build
(xo's parts pack is written on the second and read from the third); second is the second.
Reports through v0.35.0 ran xo's timed builds under XO_CACHE_STATS=1, which added about
28 MB to coll's warm peak (116 against 88 MB, 2026-11-01), so xo's peak MB steps down
from those reports to this one with no change in the compiler.
The edit's path is what xo's cache statistics say an untimed build of another edit did: separate (against
std's variants, built after the cold row as a user's second build would have them),
with the std records, or whole (the whole program).
The compile table runs both tools with their default caches (owner, 2026-11-13): xo's
run cache on, Go as is; cold empties both. Reports from 2026-10-29 to 2026-11-12 ran xo
with the run cache off (XO_BIN_CACHE=off) in the headline table, so their second and warm
rows were xo's compile path against Go's up-to-date check; that table is now the
"xo compile path (run cache off)" diagnostic below it.
Size is the default build's binary.

## Run (xo bench --ab, A = Go, B = Xo, 3 rounds)

Each pair runs the same work REPEAT times on both sides (the count is in each heading; scripts/release-vs-go.sh REPEAT), so the faster side takes about a second.

### llvm fib, repeat 5 (load 5 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go fib 5 
B: llvm-fib.xo fib 5 
4 of 4 rounds used (0 loaded), loads A/B per round: 5.86/5.71 5.71/5.71 5.71/5.41 5.41/5.41

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1026        1153        0.601% 0.313%     1.124   1.103 to 1.131    1.118 1.123     worse (n=4)
cpu_ms                1025        1151        0.586% 0.351%     1.123   1.102 to 1.13     1.117 1.122     worse (n=4)
max_rss_kb            4768        2944        2.265% 0%         0.617   0.595 to 0.626    0.604 0.624     better (n=4)
load: median 5.71 (A) / 5.56 (B); loaded: 0 of 4 rounds (at or above 8.86; baseline 5.86 + 3.0, at least 6.0)
```

### llvm sieve, repeat 6 (load 5 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go sieve 6 
B: llvm-sieve.xo sieve 6 
4 of 4 rounds used (0 loaded), loads A/B per round: 5.3/5.3 5.3/5.3 5.3/5.3 5.3/5.3

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               596.5       587.4       1.172% 8.458%     0.985   0.929 to 1.056    0.967 1.022     within noise (n=4)
cpu_ms                671.6       584.8       6.488% 8.208%     0.871   0.811 to 0.937    0.821 0.909     better (n=4)
max_rss_kb            50104       13584       0.87% 0%          0.271   0.269 to 0.276    0.27 0.274      better (n=4)
load: median 5.3 (A) / 5.3 (B); loaded: 0 of 4 rounds (at or above 8.41; baseline 5.41 + 3.0, at least 6.0)
```

### llvm nbody, repeat 6 (load 5 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go nbody 6 
B: llvm-nbody.xo nbody 6 
4 of 4 rounds used (0 loaded), loads A/B per round: 5.2/5.2 5.2/5.2 5.02/5.02 5.02/5.02

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               850.0       816.5       0.558% 0.422%     0.961   0.957 to 0.966    0.958 0.967     better (n=4)
cpu_ms                847.9       814.3       0.642% 0.425%     0.96    0.955 to 0.966    0.957 0.966     better (n=4)
max_rss_kb            4864        2928        0.493% 0%         0.602   0.602 to 0.614    0.608 0.602     better (n=4)
load: median 5.11 (A) / 5.11 (B); loaded: 0 of 4 rounds (at or above 8.2; baseline 5.2 + 3.0, at least 6.0)
```

### llvm sort, repeat 4 (load 5 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go sort 4 
B: llvm-sort.xo sort 4 
4 of 4 rounds used (0 loaded), loads A/B per round: 4.94/4.94 4.94/4.94 4.94/4.94 4.94/4.94

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               974.9       772.0       1.091% 0.398%     0.792   0.786 to 0.798    0.797 0.786     better (n=4)
cpu_ms                1023        770.1       0.951% 0.376%     0.753   0.746 to 0.767    0.759 0.747     better (n=4)
max_rss_kb            118048      27104       2.995% 0%         0.23    0.212 to 0.233    0.232 0.22      better (n=4)
load: median 4.94 (A) / 4.94 (B); loaded: 0 of 4 rounds (at or above 8.02; baseline 5.02 + 3.0, at least 6.0)
```

### llvm interp, repeat 24 (load 4 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go interp 24 
B: llvm-interp.xo interp 24 
4 of 4 rounds used (0 loaded), loads A/B per round: 4.71/4.71 4.65/4.65 4.65/4.92 4.92/4.92

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               2765        747.3       0.711% 1.601%     0.27    0.268 to 0.277    0.273 0.269     better (n=4)
cpu_ms                2762        744.6       0.707% 1.483%     0.27    0.268 to 0.277    0.273 0.269     better (n=4)
max_rss_kb            4872        2944        1.314% 0.136%     0.604   0.594 to 0.62     0.611 0.598     better (n=4)
load: median 4.68 (A) / 4.815 (B); loaded: 0 of 4 rounds (at or above 7.94; baseline 4.94 + 3.0, at least 6.0)
```

### llvm collatz, repeat 3 (load 4 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go collatz 3 
B: llvm-collatz.xo collatz 3 
4 of 4 rounds used (0 loaded), loads A/B per round: 4.68/4.68 4.39/4.39 4.39/4.2 4.2/4.2

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1318        1252        0.477% 0.795%     0.95    0.943 to 0.951    0.946 0.95      better (n=4)
cpu_ms                1315        1249        0.485% 0.761%     0.95    0.944 to 0.951    0.946 0.95      better (n=4)
max_rss_kb            4816        2944        2.159% 0%         0.611   0.592 to 0.62     0.599 0.617     better (n=4)
load: median 4.39 (A) / 4.295 (B); loaded: 0 of 4 rounds (at or above 7.68; baseline 4.68 + 3.0, at least 6.0)
```

### llvm strings, repeat 18 (load 4 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go strings 18 
B: llvm-strings.xo strings 18 
4 of 4 rounds used (0 loaded), loads A/B per round: 4.1/4.1 4.1/4.1 4.1/3.93 3.93/3.93

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1407        502.4       1.465% 1.404%     0.357   0.356 to 0.369    0.369 0.354     better (n=4)
cpu_ms                2179        499.4       1.423% 1.428%     0.229   0.228 to 0.238    0.237 0.228     better (n=4)
max_rss_kb            29336       26752       0.886% 0.03%      0.912   0.906 to 0.923    0.915 0.911     better (n=4)
load: median 4.1 (A) / 4.015 (B); loaded: 0 of 4 rounds (at or above 7.2; baseline 4.2 + 3.0, at least 6.0)
```

### coll putget, repeat 23 (load 3 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go putget 23 
B: coll.xo putget 23 
4 of 4 rounds used (0 loaded), loads A/B per round: 3.94/3.94 3.94/3.94 3.94/3.94 3.86/3.86

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1019        432.5       1.029% 1.655%     0.424   0.421 to 0.432    0.432 0.419     better (n=4)
cpu_ms                1029        430.1       1.083% 1.859%     0.418   0.414 to 0.426    0.426 0.413     better (n=4)
max_rss_kb            13104       8976        1.923% 0.089%     0.685   0.663 to 0.693    0.692 0.671     better (n=4)
load: median 3.94 (A) / 3.94 (B); loaded: 0 of 4 rounds (at or above 6.93; baseline 3.93 + 3.0, at least 6.0)
```

### coll get, repeat 52 (load 3 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go get 52 
B: coll.xo get 52 
4 of 4 rounds used (0 loaded), loads A/B per round: 3.86/3.86 3.86/3.86 3.86/3.71 3.71/3.71

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               685.3       344.7       1.487% 2.444%     0.503   0.494 to 0.545    0.5 0.525       better (n=4)
cpu_ms                684.1       343.2       1.536% 2.415%     0.502   0.492 to 0.541    0.498 0.522     better (n=4)
max_rss_kb            7712        6256        0.985% 0%         0.811   0.808 to 0.82     0.814 0.811     better (n=4)
load: median 3.86 (A) / 3.785 (B); loaded: 0 of 4 rounds (at or above 6.86; baseline 3.86 + 3.0, at least 6.0)
```

### coll push, repeat 52 (load 3 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go push 52 
B: coll.xo push 52 
4 of 4 rounds used (0 loaded), loads A/B per round: 3.71/3.71 3.71/3.71 3.71/3.71 3.71/3.71

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               241.4       191.5       2.952% 1.03%      0.793   0.762 to 0.815    0.812 0.766     better (n=4)
cpu_ms                367.0       190.2       4.809% 0.767%     0.518   0.475 to 0.539    0.531 0.49      better (n=4)
max_rss_kb            123288      29728       2.845% 0%         0.241   0.239 to 0.261    0.251 0.24      better (n=4)
load: median 3.71 (A) / 3.71 (B); loaded: 0 of 4 rounds (at or above 6.71; baseline 3.71 + 3.0, at least 6.0)
```

### coll update, repeat 57 (load 3 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go update 57 
B: coll.xo update 57 
4 of 4 rounds used (0 loaded), loads A/B per round: 3.66/3.66 3.66/3.66 3.52/3.52 3.52/3.52

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1450        577.5       0.555% 2.225%     0.398   0.39 to 0.411     0.393 0.404     better (n=4)
cpu_ms                1474        575.7       0.671% 2.139%     0.391   0.382 to 0.403    0.385 0.396     better (n=4)
max_rss_kb            13640       8976        2.551% 0.045%     0.658   0.648 to 0.685    0.655 0.669     better (n=4)
load: median 3.59 (A) / 3.59 (B); loaded: 0 of 4 rounds (at or above 6.71; baseline 3.71 + 3.0, at least 6.0)
```

### coll iter, repeat 211 (load 3 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go iter 211 
B: coll.xo iter 211 
4 of 4 rounds used (0 loaded), loads A/B per round: 3.32/3.32 3.54/3.54 3.54/3.41 3.41/3.41

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               2658        217.0       0.573% 9.734%     0.082   0.076 to 0.095    0.08 0.087      better (n=4)
cpu_ms                2656        215.7       0.538% 9.822%     0.081   0.075 to 0.095    0.079 0.087     better (n=4)
max_rss_kb            7688        6256        0.728% 0.064%     0.814   0.809 to 0.818    0.813 0.814     better (n=4)
load: median 3.475 (A) / 3.41 (B); loaded: 0 of 4 rounds (at or above 6.52; baseline 3.52 + 3.0, at least 6.0)
```

### coll copymut, repeat 134 (load 3 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go copymut 134 
B: coll.xo copymut 134 
4 of 4 rounds used (0 loaded), loads A/B per round: 3.22/3.22 3.22/3.22 3.22/3.22 3.36/3.22

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1162        341.6       1.682% 2.296%     0.294   0.276 to 0.299    0.285 0.296     better (n=4)
cpu_ms                1411        340.2       1.643% 2.356%     0.241   0.226 to 0.245    0.233 0.243     better (n=4)
max_rss_kb            13560       6256        1.593% 0%         0.461   0.457 to 0.465    0.461 0.461     better (n=4)
load: median 3.22 (A) / 3.22 (B); loaded: 0 of 4 rounds (at or above 6.41; baseline 3.41 + 3.0, at least 6.0)
```

### coll branch, repeat 90 (load 3 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go branch 90 
B: coll.xo branch 90 
4 of 4 rounds used (0 loaded), loads A/B per round: 4.52/4.72 4.72/4.72 5.73/5.56 5.56/5.56

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               22108       25.59       2.307% 3.005%     0.001   0.001 to 0.001    0.001 0.001     better (n=4)
cpu_ms                53358       24.46       2.616% 1.548%     0.0     0.0 to 0.0        0.0 0.0         better (n=4)
max_rss_kb            22512       6256        2.488% 0.064%     0.278   0.275 to 0.298    0.287 0.278     better (n=4)
load: median 5.14 (A) / 5.14 (B); loaded: 0 of 4 rounds (at or above 6.36; baseline 3.36 + 3.0, at least 6.0)
```

### conc spawn, repeat 58 (load 5 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: conc.go spawn 58 
B: conc-spawn.xo spawn 58 
4 of 4 rounds used (0 loaded), loads A/B per round: 5.32/5.38 5.38/5.38 5.35/5.4 5.4/5.4

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               3567        748.1       1.659% 1.507%     0.21    0.205 to 0.215    0.208 0.211     better (n=4)
cpu_ms                12963       759.9       1.673% 1.758%     0.059   0.057 to 0.06     0.058 0.059     better (n=4)
max_rss_kb            17032       4976        2.255% 0.804%     0.292   0.284 to 0.294    0.297 0.284     better (n=4)
load: median 5.365 (A) / 5.39 (B); loaded: 0 of 4 rounds (at or above 8.27; baseline 5.27 + 3.0, at least 6.0)
```

### conc fanout, repeat 70 (load 5 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: conc.go fanout 70 
B: conc-fanout.xo fanout 70 
4 of 4 rounds used (0 loaded), loads A/B per round: 5.53/5.56 5.56/5.56 5.56/5.52 5.52/5.52

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               2368        667.0       6.047% 0.702%     0.282   0.252 to 0.314    0.283 0.281     better (n=4)
cpu_ms                7919        671.9       4.725% 1.175%     0.085   0.078 to 0.093    0.086 0.085     better (n=4)
max_rss_kb            16120       6360        2.357% 12.89%     0.395   0.355 to 0.431    0.403 0.391     better (n=4)
load: median 5.545 (A) / 5.54 (B); loaded: 0 of 4 rounds (at or above 8.53; baseline 5.53 + 3.0, at least 6.0)
```

### conc pingpong, repeat 65 (load 5 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: conc.go pingpong 65 
B: conc-pingpong.xo pingpong 65 
4 of 4 rounds used (0 loaded), loads A/B per round: 5.48/5.2 5.2/5.2 5.2/5.2 4.86/5.2

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1238        794.5       1.692% 1.153%     0.642   0.624 to 0.653    0.641 0.64      better (n=4)
cpu_ms                1913        794.0       2.445% 1.103%     0.415   0.402 to 0.423    0.415 0.413     better (n=4)
max_rss_kb            5232        3040        1.606% 0%         0.581   0.571 to 0.586    0.574 0.586     better (n=4)
load: median 5.2 (A) / 5.2 (B); loaded: 0 of 4 rounds (at or above 8.48; baseline 5.48 + 3.0, at least 6.0)
```

### conc mutex, repeat 278 (load 4 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: conc.go mutex 278 
B: conc-mutex.xo mutex 278 
4 of 4 rounds used (0 loaded), loads A/B per round: 6.63/6.66 6.45/6.45 6.81/6.91 6.92/6.92

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               5226        1721        0.651% 1.162%     0.329   0.324 to 0.337    0.331 0.329     better (n=4)
cpu_ms                24506       3859        0.84% 1.324%      0.157   0.154 to 0.161    0.158 0.157     better (n=4)
max_rss_kb            6464        4616        1.052% 3.12%      0.714   0.659 to 0.721    0.678 0.719     better (n=4)
load: median 6.72 (A) / 6.785 (B); loaded: 0 of 4 rounds (at or above 7.86; baseline 4.86 + 3.0, at least 6.0)
```

### conc rwmutex, repeat 202 (load 5 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: conc.go rwmutex 202 
B: conc-rwmutex.xo rwmutex 202 
4 of 4 rounds used (0 loaded), loads A/B per round: 5.98/5.98 5.9/5.98 5.91/5.91 6.08/5.92

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1407        6129        0.228% 0.246%     4.356   4.339 to 4.377    4.356 4.368     worse (n=4)
cpu_ms                2839        22263       0.208% 0.528%     7.842   7.803 to 7.935    7.81 7.894      worse (n=4)
max_rss_kb            8584        6360        1.025% 4.151%     0.741   0.712 to 0.792    0.741 0.751     better (n=4)
load: median 5.945 (A) / 5.95 (B); loaded: 0 of 4 rounds (at or above 8.9; baseline 5.9 + 3.0, at least 6.0)
```

### conc pmap, repeat 19 (load 5 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: conc.go pmap 19 
B: conc-pmap.xo pmap 19 
4 of 4 rounds used (0 loaded), loads A/B per round: 5.68/5.68 5.68/5.68 5.68/5.68 5.68/5.68

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               433.2       439.0       0.49% 0.393%      1.014   0.995 to 1.018    1.003 1.015     within noise (n=4)
cpu_ms                9049        8973        0.094% 0.248%     0.992   0.99 to 0.993     0.992 0.992     better (n=4)
max_rss_kb            13328       9808        0.66% 2.814%      0.736   0.71 to 0.758     0.742 0.726     better (n=4)
load: median 5.68 (A) / 5.68 (B); loaded: 0 of 4 rounds (at or above 8.68; baseline 5.68 + 3.0, at least 6.0)
```

