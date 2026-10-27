# Xo against Go: xo 0.33.0 darwin/arm64 (2026-10-27)

scripts/release-vs-go.sh on Darwin arm64, 24 GB,
Go go1.26.1. Build times are wall milliseconds of one build; peak MB is the
build's largest resident set (cold / warm / edit). Warm is the third unchanged build
(xo's parts pack is written on the second and read from the third); second is the second.
Size is the default build's binary.

## Compile and size

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB |
|---|---|---|---|---|---|---|---|
| llvm | go | 1590 | 40 | 30 | 100 | 307 / 26 / 67 | 2.39 |
| coll | go | 1470 | 40 | 40 | 110 | 297 / 26 / 67 | 2.43 |
| conc | go | 1470 | 40 | 40 | 110 | 294 / 26 / 67 | 2.40 |
| llvm-fib | xo | 1010 | 20 | 10 | 220 | 414 / 14 / 214 | 1.60 |
| llvm-sieve | xo | 920 | 20 | 10 | 220 | 419 / 13 / 217 | 1.60 |
| llvm-nbody | xo | 920 | 20 | 20 | 220 | 417 / 14 / 218 | 1.61 |
| llvm-sort | xo | 920 | 20 | 10 | 220 | 421 / 14 / 217 | 1.61 |
| llvm-interp | xo | 940 | 20 | 10 | 230 | 405 / 14 / 215 | 1.61 |
| llvm-collatz | xo | 920 | 20 | 10 | 210 | 415 / 14 / 217 | 1.60 |
| llvm-strings | xo | 940 | 20 | 10 | 220 | 406 / 14 / 219 | 1.60 |
| coll | xo | 960 | 20 | 10 | 230 | 427 / 14 / 229 | 1.71 |
| conc-spawn | xo | 1070 | 20 | 10 | 220 | 405 / 13 / 220 | 1.63 |
| conc-fanout | xo | 1090 | 20 | 30 | 220 | 414 / 15 / 218 | 1.63 |
| conc-pingpong | xo | 1090 | 20 | 20 | 230 | 407 / 14 / 219 | 1.63 |
| conc-mutex | xo | 1080 | 20 | 10 | 220 | 409 / 14 / 216 | 1.64 |
| conc-rwmutex | xo | 1090 | 20 | 10 | 220 | 422 / 14 / 217 | 1.64 |
| conc-pmap | xo | 1080 | 20 | 10 | 220 | 413 / 14 / 220 | 1.63 |

The Go column builds one binary per group (llvm, coll, conc); the Xo
programs are one per workload except coll.

## Run (xo bench --ab, A = Go, B = Xo, 10 rounds)

### llvm fib (load 2 at start)

```
A: llvm.go fib 
B: llvm-fib.xo fib 
10 of 10 rounds used (0 loaded), loads A/B per round: 2.72/2.72 2.72/2.72 2.72/2.72 2.72/2.72 2.72/2.72 2.72/2.72 2.72/2.72 2.72/2.72 2.72/2.72 2.72/3.54

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               231.3       221.3       2.415% 1.146%     0.957   0.94 to 0.973     better (n=10)
cpu_ms                220.0       210.0       3.409% 0%         0.955   0.913 to 0.977    better (n=10)
max_rss_kb            4208        2576        0.665% 0.621%     0.612   0.61 to 0.616     better (n=10)
load: median 2.72 (A) / 2.72 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### llvm sieve (load 3 at start)

```
A: llvm.go sieve 
B: llvm-sieve.xo sieve 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.54/3.54 3.54/3.54 3.54/3.54 3.54/3.54 3.54/3.54 3.54/3.54 3.54/3.54 3.54/3.54 3.54/3.54 3.54/3.54

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               113.8       105.3       3.033% 4.331%     0.926   0.882 to 0.954    better (n=10)
cpu_ms                165.0       100.0       6.061% 10.0%      0.606   0.529 to 0.625    better (n=10)
max_rss_kb            46152       18096       2.609% 0%         0.392   0.383 to 0.395    better (n=10)
load: median 3.54 (A) / 3.54 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### llvm nbody (load 3 at start)

```
A: llvm.go nbody 
B: llvm-nbody.xo nbody 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.54/3.54 3.54/3.54 3.54/3.54 3.54/3.54 3.54/3.54 3.54/3.54 3.98/3.98 3.98/3.98 3.98/3.98 3.98/3.98

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               164.6       173.5       2.047% 1.353%     1.054   1.047 to 1.065    worse (n=10)
cpu_ms                155.0       160.0       6.452% 6.25%      1.032   1.0 to 1.067      within noise (n=10)
max_rss_kb            4240        2592        2.547% 1.543%     0.611   0.601 to 0.623    better (n=10)
load: median 3.54 (A) / 3.54 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### llvm sort (load 3 at start)

```
A: llvm.go sort 
B: llvm-sort.xo sort 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.98/3.98 3.98/3.98 3.98/3.98 3.98/3.98 3.98/3.98 3.98/3.98 3.82/3.98 3.82/3.82 3.82/3.82 3.82/3.82

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               267.9       216.0       0.91% 0.734%      0.806   0.801 to 0.809    better (n=10)
cpu_ms                270.0       210.0       0% 0%             0.778   0.778 to 0.778    better (n=10)
max_rss_kb            82968       34576       17.92% 0%         0.417   0.358 to 0.461    better (n=10)
load: median 3.98 (A) / 3.98 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### llvm interp (load 3 at start)

```
A: llvm.go interp 
B: llvm-interp.xo interp 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.82/3.82 3.82/3.82 3.82/3.82 3.82/3.82 3.82/3.82 3.82/3.82 3.82/3.82 3.82/3.82 3.82/3.82 3.82/3.82

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               146.3       73.08       1.418% 2.165%     0.5     0.495 to 0.508    better (n=10)
cpu_ms                140.0       60.0        0% 16.67%         0.429   0.429 to 0.5      better (n=10)
max_rss_kb            4248        2576        1.036% 0.466%     0.606   0.603 to 0.61     better (n=10)
load: median 3.82 (A) / 3.82 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### llvm collatz (load 3 at start)

```
A: llvm.go collatz 
B: llvm-collatz.xo collatz 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.6/3.6 3.6/3.6 3.6/3.6 3.6/3.6 3.6/3.6 3.6/3.47 3.47/3.47 3.47/3.47 3.47/3.47 3.47/3.47

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               467.7       448.7       0.48% 0.882%      0.96    0.952 to 0.966    better (n=10)
cpu_ms                460.0       440.0       0% 0%             0.957   0.946 to 0.957    better (n=10)
max_rss_kb            4208        2576        1.426% 1.398%     0.612   0.608 to 0.627    better (n=10)
load: median 3.6 (A) / 3.535 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### llvm strings (load 3 at start)

```
A: llvm.go strings 
B: llvm-strings.xo strings 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.47/3.47 3.47/3.47 3.35/3.47 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               89.96       41.54       1.924% 2.525%     0.462   0.448 to 0.47     better (n=10)
cpu_ms                120.0       30.0        8.333% 0%         0.25    0.25 to 0.273     better (n=10)
max_rss_kb            23680       15248       5.49% 0%          0.644   0.616 to 0.664    better (n=10)
load: median 3.35 (A) / 3.35 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### coll putget (load 3 at start)

```
A: coll.go putget 
B: coll.xo putget 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               51.38       29.87       2.568% 3.37%      0.581   0.567 to 0.596    better (n=10)
cpu_ms                40.0        20.0        0% 0%             0.5     0.5 to 0.5        too coarse (n=10)
max_rss_kb            8728        7264        2.154% 0%         0.832   0.824 to 0.847    better (n=10)
load: median 3.35 (A) / 3.35 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### coll get (load 3 at start)

```
A: coll.go get 
B: coll.xo get 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.94       13.81       2.334% 1.475%     0.602   0.592 to 0.613    better (n=10)
cpu_ms                20.0        10.0        37.5% 0%          0.5     0.5 to 1.0        too coarse (n=10)
max_rss_kb            7080        6864        1.469% 0%         0.969   0.958 to 0.974    better (n=10)
load: median 3.35 (A) / 3.35 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### coll push (load 3 at start)

```
A: coll.go push 
B: coll.xo push 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               12.19       9.054       4.125% 2.689%     0.743   0.715 to 0.757    better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            54632       50912       7.995% 0%         0.932   0.795 to 0.941    better (n=10)
load: median 3.35 (A) / 3.35 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### coll update (load 3 at start)

```
A: coll.go update 
B: coll.xo update 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               31.24       19.17       2.048% 2.707%     0.613   0.603 to 0.628    better (n=10)
cpu_ms                20.0        10.0        0% 0%             0.5     0.5 to 0.5        too coarse (n=10)
max_rss_kb            8696        7264        3.588% 0%         0.835   0.823 to 0.857    better (n=10)
load: median 3.35 (A) / 3.35 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### coll iter (load 3 at start)

```
A: coll.go iter 
B: coll.xo iter 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.57       6.257       2.142% 12.69%     0.277   0.263 to 0.326    better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            7096        6864        1.071% 0%         0.967   0.962 to 0.973    better (n=10)
load: median 3.35 (A) / 3.35 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### coll copymut (load 3 at start)

```
A: coll.go copymut 
B: coll.xo copymut 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.35

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               14.19       9.378       0.833% 1.466%     0.661   0.652 to 0.667    better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            10016       6880        2.037% 0%         0.687   0.671 to 0.689    better (n=10)
load: median 3.35 (A) / 3.35 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### coll branch (load 3 at start)

```
A: coll.go branch 
B: coll.xo branch 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.35/3.35 3.35/3.35 3.35/3.35 3.35/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               205.1       5.137       2.068% 3.918%     0.025   0.024 to 0.027    better (n=10)
cpu_ms                310.0       0           4.839% 0%         0       0 to 0            better (n=10)
max_rss_kb            15280       6912        6.466% 0.694%     0.452   0.434 to 0.47     better (n=10)
load: median 3.64 (A) / 3.64 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### conc spawn (load 3 at start)

```
A: conc.go spawn 
B: conc-spawn.xo spawn 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               50.46       23.38       2.544% 4.161%     0.463   0.444 to 0.475    better (n=10)
cpu_ms                150.0       20.0        11.67% 0%         0.133   0.118 to 0.167    better (n=10)
max_rss_kb            11128       5952        2.157% 9.005%     0.535   0.491 to 0.565    better (n=10)
load: median 3.64 (A) / 3.64 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### conc fanout (load 3 at start)

```
A: conc.go fanout 
B: conc-fanout.xo fanout 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.8        16.06       6.847% 2.462%     0.704   0.659 to 0.739    better (n=10)
cpu_ms                50.0        10.0        15.0% 0%          0.2     0.167 to 0.2      too coarse (n=10)
max_rss_kb            11752       4416        5.003% 2.264%     0.376   0.368 to 0.399    better (n=10)
load: median 3.64 (A) / 3.64 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### conc pingpong (load 3 at start)

```
A: conc.go pingpong 
B: conc-pingpong.xo pingpong 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               21.07       25.2        5.876% 3.273%     1.196   1.146 to 1.229    worse (n=10)
cpu_ms                10.0        20.0        0% 0%             2.0     2.0 to 2.0        too coarse (n=10)
max_rss_kb            4240        2656        2.358% 0.602%     0.626   0.618 to 0.637    better (n=10)
load: median 3.64 (A) / 3.64 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### conc mutex (load 3 at start)

```
A: conc.go mutex 
B: conc-mutex.xo mutex 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               23.71       6.734       2.265% 1.956%     0.284   0.279 to 0.292    better (n=10)
cpu_ms                90.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            4752        2576        1.178% 0%         0.542   0.536 to 0.544    better (n=10)
load: median 3.64 (A) / 3.64 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### conc rwmutex (load 3 at start)

```
A: conc.go rwmutex 
B: conc-rwmutex.xo rwmutex 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               10.52       9.139       3.381% 3.11%      0.869   0.82 to 0.897     better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            4792        2592        4.007% 0%         0.541   0.521 to 0.545    better (n=10)
load: median 3.64 (A) / 3.64 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

### conc pmap (load 3 at start)

```
A: conc.go pmap 
B: conc-pmap.xo pmap 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.64 3.64/3.51 3.51/3.51 3.51/3.51

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               56.92       61.81       1.839% 5.956%     1.086   1.072 to 1.172    worse (n=10)
cpu_ms                540.0       590.0       1.389% 0%         1.093   1.073 to 1.093    worse (n=10)
max_rss_kb            6792        5360        1.06% 1.567%      0.789   0.774 to 0.794    better (n=10)
load: median 3.64 (A) / 3.64 (B); loaded: 0 of 10 rounds (at or above 6.0)
```

