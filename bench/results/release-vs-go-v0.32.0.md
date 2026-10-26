# Xo against Go: xo 0.32.0 darwin/arm64 (2026-10-25)

scripts/release-vs-go.sh on Darwin arm64, 24 GB,
Go go1.26.1. Build times are wall milliseconds of one build; peak MB is the
build's largest resident set (cold / warm / edit). Warm is the third unchanged build
(xo's parts pack is written on the second and read from the third); second is the second.
Size is the default build's binary.

## Compile and size

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB |
|---|---|---|---|---|---|---|---|
| llvm | go | 1500 | 40 | 40 | 100 | 313 / 25 / 67 | 2.39 |
| coll | go | 1450 | 40 | 40 | 110 | 312 / 25 / 68 | 2.43 |
| conc | go | 1450 | 30 | 30 | 100 | 307 / 26 / 68 | 2.40 |
| llvm-fib | xo | 1120 | 40 | 30 | 340 | 507 / 14 / 301 | 1.72 |
| llvm-sieve | xo | 1110 | 40 | 30 | 340 | 506 / 14 / 306 | 1.72 |
| llvm-nbody | xo | 1110 | 30 | 30 | 330 | 519 / 14 / 300 | 1.72 |
| llvm-sort | xo | 1090 | 30 | 30 | 330 | 499 / 14 / 303 | 1.72 |
| llvm-interp | xo | 1180 | 30 | 30 | 360 | 508 / 14 / 305 | 1.72 |
| llvm-collatz | xo | 1110 | 30 | 30 | 330 | 493 / 14 / 302 | 1.72 |
| llvm-strings | xo | 1100 | 30 | 30 | 340 | 494 / 14 / 299 | 1.72 |
| coll | xo | 1140 | 30 | 30 | 360 | 502 / 14 / 317 | 1.79 |
| conc-spawn | xo | 1240 | 30 | 30 | 350 | 509 / 14 / 299 | 1.74 |
| conc-fanout | xo | 1250 | 30 | 30 | 330 | 510 / 14 / 305 | 1.74 |
| conc-pingpong | xo | 1290 | 30 | 30 | 340 | 490 / 14 / 302 | 1.74 |
| conc-mutex | xo | 1250 | 30 | 30 | 340 | 495 / 14 / 302 | 1.74 |
| conc-rwmutex | xo | 1240 | 30 | 30 | 340 | 511 / 14 / 307 | 1.74 |
| conc-pmap | xo | 1250 | 40 | 30 | 340 | 499 / 14 / 301 | 1.74 |

The Go column builds one binary per group (llvm, coll, conc); the Xo
programs are one per workload except coll.

## Run (xo bench --ab, A = Go, B = Xo, 10 rounds)

### llvm fib (load 4 at start)

```
A: llvm.go fib 
B: llvm-fib.xo fib 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.72/4.72 4.72/4.72 4.72/4.72 4.72/4.72 4.72/4.72 4.72/4.72 4.72/4.72 4.72/4.72 4.59/4.59 4.59/4.59

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               226.0       224.1       2.03% 2.729%      0.991   0.973 to 1.01     within noise (n=10)
cpu_ms                220.0       220.0       0% 4.545%         1.0     0.955 to 1.0      within noise (n=10)
max_rss_kb            4200        2544        0.667% 0%         0.606   0.6 to 0.609      better (n=10)
```

### llvm sieve (load 4 at start)

```
A: llvm.go sieve 
B: llvm-sieve.xo sieve 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.59/4.59 4.59/4.59 4.59/4.59 4.59/4.59 4.59/4.59 4.59/4.59 4.59/4.59 4.59/4.59 4.59/4.59 4.59/4.59

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               112.2       102.0       1.637% 10.61%     0.91    0.889 to 1.019    within noise (n=10)
cpu_ms                170.0       90.0        5.882% 19.44%     0.529   0.529 to 0.647    better (n=10)
max_rss_kb            44552       18096       3.528% 0.088%     0.406   0.394 to 0.413    better (n=10)
```

### llvm nbody (load 4 at start)

```
A: llvm.go nbody 
B: llvm-nbody.xo nbody 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.59/4.59 4.59/4.59 4.3/4.3 4.3/4.3 4.3/4.3 4.3/4.3 4.3/4.3 4.3/4.3 4.3/4.3 4.3/4.3

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               162.8       231.6       0.673% 0.242%     1.423   1.414 to 1.427    worse (n=10)
cpu_ms                150.0       220.0       0% 0%             1.467   1.467 to 1.467    worse (n=10)
max_rss_kb            4240        2560        0.283% 1.563%     0.604   0.596 to 0.615    better (n=10)
```

### llvm sort (load 4 at start)

```
A: llvm.go sort 
B: llvm-sort.xo sort 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.3/4.3 4.3/4.3 4.3/4.3 4.19/4.19 4.19/4.19 4.19/4.19 4.19/4.19 4.19/4.19 4.19/4.19 4.19/4.19

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               267.0       214.8       0.576% 0.142%     0.805   0.801 to 0.807    better (n=10)
cpu_ms                270.0       210.0       0% 0%             0.778   0.778 to 0.778    better (n=10)
max_rss_kb            76096       34576       20.54% 0.116%     0.454   0.41 to 0.542     better (n=10)
```

### llvm interp (load 4 at start)

```
A: llvm.go interp 
B: llvm-interp.xo interp 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.19/4.19 4.19/4.19 4.19/4.19 4.19/4.19 4.19/4.19 4.02/4.02 4.02/4.02 4.02/4.02 4.02/4.02 4.02/4.02

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               146.1       59.57       1.131% 1.043%     0.408   0.404 to 0.412    better (n=10)
cpu_ms                140.0       50.0        0% 0%             0.357   0.357 to 0.357    better (n=10)
max_rss_kb            4184        2560        2.581% 0%         0.612   0.602 to 0.618    better (n=10)
```

### llvm collatz (load 4 at start)

```
A: llvm.go collatz 
B: llvm-collatz.xo collatz 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.02/4.02 4.02/4.02 4.02/4.02 3.78/3.78 3.78/3.78 3.78/3.78 3.78/3.78 3.78/3.78 3.55/3.78 3.55/3.55

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               492.5       472.9       0.327% 0.68%      0.96    0.957 to 0.966    better (n=10)
cpu_ms                480.0       460.0       0% 0%             0.958   0.958 to 0.969    better (n=10)
max_rss_kb            4208        2544        2.186% 0%         0.605   0.6 to 0.615      better (n=10)
```

### llvm strings (load 3 at start)

```
A: llvm.go strings 
B: llvm-strings.xo strings 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.55/3.55 3.55/3.55 3.55/3.55 3.55/3.55 3.55/3.55 3.55/3.55 3.55/3.55 3.55/3.55 3.55/3.55 3.55/3.55

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               90.24       40.78       1.89% 3.126%      0.452   0.438 to 0.459    better (n=10)
cpu_ms                115.0       30.0        8.696% 0%         0.261   0.25 to 0.273     better (n=10)
max_rss_kb            24768       15232       7.558% 0%         0.615   0.581 to 0.658    better (n=10)
```

### coll putget (load 3 at start)

```
A: coll.go putget 
B: coll.xo putget 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.55/3.55 3.55/3.55 3.55/3.55 3.55/3.55 3.55/3.55 3.55/3.55 3.55/3.55 3.55/3.55 3.55/3.55 3.55/3.55

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               50.21       29.58       2.567% 2.681%     0.589   0.572 to 0.595    better (n=10)
cpu_ms                40.0        20.0        0% 0%             0.5     0.5 to 0.5        too coarse (n=10)
max_rss_kb            8768        7264        1.551% 0.165%     0.828   0.825 to 0.851    better (n=10)
```

### coll get (load 3 at start)

```
A: coll.go get 
B: coll.xo get 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.55/3.55 3.55/3.55 3.55/3.55 3.55/3.55 3.55/3.55 3.55/3.55 3.55/3.55 3.51/3.51 3.51/3.51 3.51/3.51

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.54       13.81       1.647% 2.029%     0.613   0.604 to 0.621    better (n=10)
cpu_ms                10.0        10.0        100.0% 0%         1.0     0.5 to 1.0        too coarse (n=10)
max_rss_kb            7144        6912        0.56% 0.463%      0.968   0.961 to 0.972    better (n=10)
```

### coll push (load 3 at start)

```
A: coll.go push 
B: coll.xo push 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               12.47       8.994       4.39% 3.658%      0.721   0.707 to 0.752    better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            68552       50920       17.89% 0.079%     0.743   0.724 to 0.904    better (n=10)
```

### coll update (load 3 at start)

```
A: coll.go update 
B: coll.xo update 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               31.37       19.94       1.37% 1.246%      0.636   0.629 to 0.64     better (n=10)
cpu_ms                20.0        10.0        0% 0%             0.5     0.5 to 0.5        too coarse (n=10)
max_rss_kb            8816        7312        0.726% 0.438%     0.829   0.822 to 0.831    better (n=10)
```

### coll iter (load 3 at start)

```
A: coll.go iter 
B: coll.xo iter 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               21.98       5.563       2.056% 6.921%     0.253   0.247 to 0.269    better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            7096        6880        0.846% 0.64%      0.97    0.962 to 0.974    better (n=10)
```

### coll copymut (load 3 at start)

```
A: coll.go copymut 
B: coll.xo copymut 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               14.16       8.767       1.381% 0.958%     0.619   0.613 to 0.624    better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            10104       6880        3.603% 0.233%     0.681   0.664 to 0.691    better (n=10)
```

### coll branch (load 3 at start)

```
A: coll.go branch 
B: coll.xo branch 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               203.1       4.97        3.59% 4.945%      0.024   0.024 to 0.026    better (n=10)
cpu_ms                300.0       0           3.333% 0%         0       0 to 0            better (n=10)
max_rss_kb            14872       6880        5.998% 0%         0.463   0.45 to 0.479     better (n=10)
```

### conc spawn (load 3 at start)

```
A: conc.go spawn 
B: conc-spawn.xo spawn 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.79/3.79

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               48.77       22.72       4.655% 2.116%     0.466   0.448 to 0.479    better (n=10)
cpu_ms                150.0       20.0        0% 0%             0.133   0.1 to 0.133      better (n=10)
max_rss_kb            11200       5552        0.893% 16.07%     0.496   0.443 to 0.537    better (n=10)
```

### conc fanout (load 3 at start)

```
A: conc.go fanout 
B: conc-fanout.xo fanout 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.85       16.6        8.301% 4.625%     0.726   0.681 to 0.778    better (n=10)
cpu_ms                50.0        10.0        15.0% 0%          0.2     0.167 to 0.2      too coarse (n=10)
max_rss_kb            11656       4480        2.848% 14.2%      0.384   0.372 to 0.432    better (n=10)
```

### conc pingpong (load 3 at start)

```
A: conc.go pingpong 
B: conc-pingpong.xo pingpong 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.29       26.49       20.92% 2.633%     1.188   0.998 to 1.236    within noise (n=10)
cpu_ms                10.0        20.0        200.0% 0%         2.0     0.667 to 2.0      too coarse (n=10)
max_rss_kb            4256        2656        1.128% 1.205%     0.624   0.618 to 0.634    better (n=10)
```

### conc mutex (load 3 at start)

```
A: conc.go mutex 
B: conc-mutex.xo mutex 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               23.13       7.055       2.115% 7.361%     0.305   0.292 to 0.326    better (n=10)
cpu_ms                90.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            4784        2616        1.003% 1.835%     0.547   0.543 to 0.559    better (n=10)
```

### conc rwmutex (load 3 at start)

```
A: conc.go rwmutex 
B: conc-rwmutex.xo rwmutex 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               10.16       9.835       6.043% 10.85%     0.968   0.902 to 1.032    within noise (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            4912        2632        3.176% 1.52%      0.536   0.525 to 0.545    better (n=10)
```

### conc pmap (load 3 at start)

```
A: conc.go pmap 
B: conc-pmap.xo pmap 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79 3.79/3.79

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               55.78       60.01       1.364% 0.489%     1.076   1.066 to 1.086    worse (n=10)
cpu_ms                550.0       590.0       1.818% 0%         1.073   1.073 to 1.093    worse (n=10)
max_rss_kb            6848        5360        1.636% 0.896%     0.783   0.771 to 0.793    better (n=10)
```

