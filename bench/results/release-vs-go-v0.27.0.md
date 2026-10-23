# Xo against Go: xo 0.27.0 darwin/arm64 (2026-10-23)

Release step 7 (toolchain.md): the released darwin/arm64 archive of
v0.27.0 (public commit b6cfa83ce, xo stamped with it), unpacked, against
Go 1.26.1, by scripts/release-vs-go.sh in a bulk slot on the development
Mac (load 4 to 5 at the start of each run, other agents running): `--ab`'s
verdicts account for the spread of its 10 rounds.

Summary:

- Run time, Xo over Go (wall, median): Xo is faster on 14 of 20
  workloads (fib 0.96, sieve 0.97, collatz 0.96, sort 0.81, interp 0.42,
  strings 0.44, every collection workload 0.03 to 0.72, conc mutex 0.46),
  within noise on 2 (conc fanout 0.89, conc pingpong 1.12), and slower on
  4: conc rwmutex 3.58, llvm nbody 1.57, conc spawn 1.20, conc pmap 1.07.
- Peak memory of the run: Xo lower on all 20 (0.38 to 0.99 of Go's).
- Binary size: 1.85 to 1.94 MB against Go's 2.39 to 2.43 MB.
- Compile: cold builds 1.8 to 2.3 s against Go's 1.6 to 1.9 s; warm
  builds 0.27 to 0.34 s against 0.04 s (0.42 to 0.64 s in 0.26.0), and a
  one function edit 0.27 to 0.36 s against 0.11 to 0.14 s; xo's build
  peaks at 0.49 to 0.50 GB cold and 0.30 to 0.31 GB warm (0.76 to 0.89 GB
  and 0.78 GB in 0.26.0) against Go's 0.30 to 0.33 GB cold (25 to 26 MB
  warm). A warm build of an unchanged program is still xo's largest gap.

scripts/release-vs-go.sh on Darwin arm64, 24 GB,
Go go1.26.1. Build times are wall milliseconds of one build; peak MB is the
build's largest resident set (cold / warm / edit). Warm is the third unchanged build
(xo's parts pack is written on the second and read from the third); second is the second.
Size is the default build's binary.

## Compile and size

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB |
|---|---|---|---|---|---|---|---|
| llvm | go | 1890 | 40 | 40 | 110 | 331 / 26 / 67 | 2.39 |
| coll | go | 1640 | 40 | 40 | 140 | 300 / 25 / 66 | 2.43 |
| conc | go | 1670 | 40 | 40 | 110 | 315 / 26 / 69 | 2.40 |
| llvm-fib | xo | 2060 | 430 | 340 | 360 | 496 / 307 / 303 | 1.85 |
| llvm-sieve | xo | 1990 | 370 | 280 | 290 | 492 / 300 / 305 | 1.85 |
| llvm-nbody | xo | 1850 | 360 | 280 | 280 | 495 / 303 / 305 | 1.87 |
| llvm-sort | xo | 1870 | 360 | 290 | 270 | 491 / 304 / 307 | 1.87 |
| llvm-interp | xo | 1820 | 350 | 280 | 270 | 494 / 305 / 304 | 1.87 |
| llvm-collatz | xo | 1830 | 360 | 270 | 270 | 496 / 300 / 306 | 1.85 |
| llvm-strings | xo | 2080 | 370 | 280 | 290 | 494 / 307 / 301 | 1.85 |
| coll | xo | 1920 | 380 | 290 | 300 | 502 / 309 / 313 | 1.94 |
| conc-spawn | xo | 2090 | 380 | 320 | 300 | 494 / 303 / 307 | 1.87 |
| conc-fanout | xo | 2310 | 400 | 290 | 310 | 495 / 302 / 303 | 1.87 |
| conc-pingpong | xo | 1910 | 350 | 270 | 270 | 485 / 300 / 305 | 1.89 |
| conc-mutex | xo | 2250 | 420 | 300 | 310 | 491 / 306 / 306 | 1.87 |
| conc-rwmutex | xo | 2100 | 350 | 280 | 270 | 493 / 302 / 305 | 1.87 |
| conc-pmap | xo | 1920 | 340 | 290 | 280 | 491 / 304 / 303 | 1.87 |

The Go column builds one binary per group (llvm, coll, conc); the Xo
programs are one per workload except coll.

## Run (xo bench --ab, A = Go, B = Xo, 10 rounds)

### llvm fib (load 5 at start)

```
A: llvm.go fib 
B: llvm-fib.xo fib 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.4/5.44 5.4/5.4 5.4/5.4 5.4/5.4 5.4/5.4 5.4/5.4 5.4/5.4 5.4/5.4 5.4/5.4 5.4/5.4

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               247.7       236.9       3.345% 4.362%     0.957   0.929 to 0.995    better (n=10)
cpu_ms                230.0       220.0       0% 0%             0.957   0.957 to 0.957    better (n=10)
max_rss_kb            4208        2616        1.331% 1.223%     0.622   0.612 to 0.627    better (n=10)
```

### llvm sieve (load 5 at start)

```
A: llvm.go sieve 
B: llvm-sieve.xo sieve 
10 of 10 rounds used (0 loaded), loads A/B per round: 6.73/6.73 6.73/6.73 6.73/6.73 6.73/6.73 6.73/6.73 6.73/6.73 6.73/6.73 6.73/6.73 6.73/6.73 6.73/6.73

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               123.8       120.4       5.202% 7.114%     0.973   0.896 to 0.996    better (n=10)
cpu_ms                150.0       110.0       5.0% 6.818%       0.733   0.677 to 0.759    better (n=10)
max_rss_kb            47528       18160       3.905% 0.132%     0.382   0.371 to 0.39     better (n=10)
```

### llvm nbody (load 5 at start)

```
A: llvm.go nbody 
B: llvm-nbody.xo nbody 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.83/5.83 5.83/5.83 5.83/5.83 5.83/5.83 6.73/6.73 6.73/6.73 6.73/6.73 6.73/6.73 6.73/6.73 6.73/6.73

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               172.0       269.9       2.535% 1.587%     1.569   1.522 to 1.591    worse (n=10)
cpu_ms                160.0       260.0       0% 2.885%         1.625   1.563 to 1.625    worse (n=10)
max_rss_kb            4248        2648        2.26% 1.208%      0.623   0.618 to 0.637    better (n=10)
```

### llvm sort (load 5 at start)

```
A: llvm.go sort 
B: llvm-sort.xo sort 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.28/5.28 5.28/5.28 5.28/5.28 5.28/6.78 6.78/6.78 6.78/6.78 6.78/6.78 6.78/6.78 6.78/6.78 6.78/6.78

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               282.9       229.0       1.593% 2.1%       0.81    0.792 to 0.817    better (n=10)
cpu_ms                280.0       210.0       0% 0%             0.75    0.75 to 0.768     better (n=10)
max_rss_kb            89912       34640       23.68% 0.092%     0.385   0.351 to 0.466    better (n=10)
```

### llvm interp (load 5 at start)

```
A: llvm.go interp 
B: llvm-interp.xo interp 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.75/5.75 5.75/5.75 5.75/5.75 5.75/5.75 5.53/5.75 5.53/5.53 5.53/5.53 5.53/5.53 5.53/5.53 5.53/5.53

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               148.5       62.15       3.771% 2.056%     0.418   0.408 to 0.428    better (n=10)
cpu_ms                140.0       50.0        0% 0%             0.357   0.357 to 0.393    better (n=10)
max_rss_kb            4232        2648        1.607% 0.604%     0.626   0.623 to 0.635    better (n=10)
```

### llvm collatz (load 5 at start)

```
A: llvm.go collatz 
B: llvm-collatz.xo collatz 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.53/5.53 5.53/5.53 5.53/5.53 5.24/5.24 5.24/5.24 5.24/5.24 5.24/5.24 5.24/5.24 4.98/4.98 4.98/4.98

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               508.4       487.8       1.726% 1.136%     0.959   0.949 to 0.968    better (n=10)
cpu_ms                500.0       480.0       1.5% 1.563%       0.96    0.94 to 0.98      better (n=10)
max_rss_kb            4160        2592        2.404% 0%         0.623   0.609 to 0.625    better (n=10)
```

### llvm strings (load 4 at start)

```
A: llvm.go strings 
B: llvm-strings.xo strings 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.98/4.98 4.98/4.98 4.98/4.98 4.98/4.98 4.98/4.98 4.98/4.98 4.98/4.98 4.98/4.98 4.98/4.98 4.98/4.98

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               101.1       44.57       1.134% 3.338%     0.441   0.431 to 0.451    better (n=10)
cpu_ms                130.0       35.0        0% 28.57%         0.269   0.231 to 0.308    better (n=10)
max_rss_kb            24864       15280       3.748% 0.183%     0.615   0.588 to 0.638    better (n=10)
```

### coll putget (load 4 at start)

```
A: coll.go putget 
B: coll.xo putget 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.98/4.98 4.98/4.98 4.98/4.98 4.98/6.35 6.35/6.35 6.35/6.35 6.35/6.35 6.35/6.35 6.35/6.35 6.35/6.35

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               53.52       32.23       2.544% 2.415%     0.602   0.575 to 0.619    better (n=10)
cpu_ms                40.0        20.0        18.75% 0%         0.5     0.4 to 0.5        too coarse (n=10)
max_rss_kb            8776        7504        1.185% 0.373%     0.855   0.844 to 0.861    better (n=10)
```

### coll get (load 5 at start)

```
A: coll.go get 
B: coll.xo get 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               24.12       15.58       4.729% 8.474%     0.646   0.599 to 0.678    better (n=10)
cpu_ms                20.0        10.0        0% 0%             0.5     0.5 to 0.5        too coarse (n=10)
max_rss_kb            7080        6992        0.621% 0.858%     0.988   0.981 to 0.993    better (n=10)
```

### coll push (load 5 at start)

```
A: coll.go push 
B: coll.xo push 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               14.11       10.18       5.38% 9.078%      0.721   0.691 to 0.777    better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            58792       51056       21.05% 0.11%      0.868   0.727 to 0.939    better (n=10)
```

### coll update (load 5 at start)

```
A: coll.go update 
B: coll.xo update 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               33.36       21.22       4.373% 6.191%     0.636   0.616 to 0.671    better (n=10)
cpu_ms                20.0        10.0        0% 0%             0.5     0.5 to 0.5        too coarse (n=10)
max_rss_kb            8776        7520        1.778% 0.319%     0.857   0.828 to 0.869    better (n=10)
```

### coll iter (load 5 at start)

```
A: coll.go iter 
B: coll.xo iter 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               24.26       6.469       4.511% 7.506%     0.267   0.25 to 0.283     better (n=10)
cpu_ms                20.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            7096        6960        1.24% 0.575%      0.981   0.974 to 0.989    better (n=10)
```

### coll copymut (load 5 at start)

```
A: coll.go copymut 
B: coll.xo copymut 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               15.56       9.841       1.377% 12.28%     0.633   0.598 to 0.685    better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            10136       6976        3.275% 0.688%     0.688   0.669 to 0.7      better (n=10)
```

### coll branch (load 5 at start)

```
A: coll.go branch 
B: coll.xo branch 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/5.89 5.89/7.1 7.1/7.1 7.1/7.1

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               237.7       5.916       102.4% 20.8%      0.025   0.012 to 0.028    better (n=10)
cpu_ms                380.0       0           11.18% 0%         0       0 to 0            better (n=10)
max_rss_kb            15272       6960        344.9% 0.23%      0.456   0.102 to 0.485    better (n=10)
```

### conc spawn (load 5 at start)

```
A: conc.go spawn 
B: conc-spawn.xo spawn 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.64/5.64 5.64/5.64 5.64/5.64 5.64/5.64 5.64/5.64 5.64/5.64 5.64/5.64 5.64/5.64 5.64/5.64 5.64/5.64

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               50.52       60.43       15.53% 5.523%     1.196   1.022 to 1.273    worse (n=10)
cpu_ms                145.0       240.0       20.69% 4.167%     1.655   1.371 to 1.714    worse (n=10)
max_rss_kb            11208       7400        3.569% 6.054%     0.66    0.585 to 0.682    better (n=10)
```

### conc fanout (load 5 at start)

```
A: conc.go fanout 
B: conc-fanout.xo fanout 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.64/5.64 5.64/5.64 5.64/5.64 5.64/5.64 5.64/5.64 5.64/5.64 5.64/5.64 5.64/5.64 5.64/5.64 5.64/5.64

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               32.43       28.92       16.88% 12.35%     0.892   0.814 to 1.017    within noise (n=10)
cpu_ms                70.0        95.0        14.29% 10.53%     1.357   1.125 to 1.429    too coarse (n=10)
max_rss_kb            11976       7416        10.55% 4.369%     0.619   0.548 to 0.626    better (n=10)
```

### conc pingpong (load 5 at start)

```
A: conc.go pingpong 
B: conc-pingpong.xo pingpong 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.64/5.64 5.64/5.64 5.64/5.64 5.64/5.64 5.64/5.64 5.64/6.71 6.71/6.71 6.71/6.71 6.71/6.71 6.71/6.71

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               34.21       38.32       27.81% 43.7%      1.12    0.868 to 1.437    within noise (n=10)
cpu_ms                15.0        20.0        66.67% 0%         1.333   0.8 to 2.0        too coarse (n=10)
max_rss_kb            4272        2752        2.154% 10.17%     0.644   0.626 to 0.709    better (n=10)
```

### conc mutex (load 5 at start)

```
A: conc.go mutex 
B: conc-mutex.xo mutex 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               23.64       10.78       1.593% 3.523%     0.456   0.441 to 0.472    better (n=10)
cpu_ms                90.0        10.0        0% 0%             0.111   0.111 to 0.111    too coarse (n=10)
max_rss_kb            4776        3384        2.68% 4.965%      0.709   0.677 to 0.723    better (n=10)
```

### conc rwmutex (load 5 at start)

```
A: conc.go rwmutex 
B: conc-rwmutex.xo rwmutex 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               9.962       35.63       6.028% 1.588%     3.577   3.482 to 3.714    worse (n=10)
cpu_ms                10.0        100.0       75.0% 10.0%       10.0    9.5 to 22.0       worse (n=10)
max_rss_kb            4808        4472        2.246% 5.903%     0.93    0.889 to 0.97     better (n=10)
```

### conc pmap (load 5 at start)

```
A: conc.go pmap 
B: conc-pmap.xo pmap 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28 5.28/5.28

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               57.96       61.96       1.548% 2.748%     1.069   1.059 to 1.097    worse (n=10)
cpu_ms                540.0       580.0       0% 1.293%         1.074   1.074 to 1.093    worse (n=10)
max_rss_kb            6792        5432        1.119% 0.736%     0.8     0.793 to 0.81     better (n=10)
```

