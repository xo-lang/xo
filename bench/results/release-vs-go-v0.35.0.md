# Xo against Go: xo 0.35.0 darwin/arm64 (2026-10-30)

The release comparison (toolchain.md "Cutting a release" step 7) on the
released darwin/arm64 archive of v0.35.0 (216203685, the 0.35.0 tag after
the Linux run cache fix was folded in), scripts/release-vs-go.sh from the
tag's tree, run as HEAVY_CLASS=measure (a first run as bulk had every
`xo bench --ab` refuse; 71cd6fb43 fixed the docs). Every run started at load
4 or 5. The comparison against the previous release's report
(release-vs-go-v0.34.0.md) follows the script's report: each run row's B/A
ratio (Xo over Go) old against new, a row worse by more than 3% flagged.

## Against 0.34.0

Wall ratios against Go: 18 of 20 workloads faster than Go, nbody slower
(1.059; 0.34.0: 1.040) and pmap within noise (0.988; 0.34.0: 1.010).
Flagged wall rows: interp 0.291 to 0.321 (+10.3%), coll iter 0.264 to 0.274
(+3.8%), coll get 0.599 to 0.618 (+3.2%). interp's B spread is 18%, so a
direct A/B of interp built by the 0.34.0 and 0.35.0 archives (same source,
20 rounds, load 5.87, private caches) decides it: wall 1.028, 0.953 to
1.096, within noise; max_rss 2592 to 2608 KB (1.006). iter and get move 3
to 4% on 6 to 23 ms medians. Better: collatz 0.965 to 0.898, copymut 0.675
to 0.632, pingpong 0.897 to 0.873, pmap 1.010 to 0.988. cpu rows flagged
(fib 0.955 to 1.000, interp 0.250 to 0.286) are 10 ms granularity.
Compile: xo's edit builds 60 to 70 ms (coll 130) against Go's 100 to 110,
every edit on the std-record path; cold 880 to 1100 ms against Go's 1430 to
1520.

Run cache row (owner, 2026-10-29: the report's rows run with the run cache
off): an unchanged rebuild with the run cache on (the default), the released
xo, five rebuilds after two warm-up builds, `/usr/bin/time -l` of
`xo build -o OUT .` (10 ms resolution): llvm-fib 10 ms and 15 MB on all five,
coll 10 ms and 7 MB on all five. `go build -o OUT .` in the same harness: llvm
120 to 130 ms and 25 to 26 MB, coll 120 ms and 25 to 26 MB (the compile table's
Go second-build 40 ms comes from the script's own timing, a different
harness).

Direct interp A/B (A = built by 0.34.0, B = built by 0.35.0):

```
metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               42.43       43.61       7.243% 11.29%     1.028   0.953 to 1.096    within noise (n=20)
cpu_ms                30.0        40.0        33.33% 25.0%      1.333   0.857 to 1.333    too coarse (n=20)
max_rss_kb            2592        2608        0.154% 0.46%      1.006   1.006 to 1.006    worse (n=20)
```

| workload | metric | 0.34.0 B/A | 0.35.0 B/A | change |
|---|---|---|---|---|
| llvm fib | wall_ms | 0.966 | 0.965 | -0.1% |
| llvm fib | cpu_ms | 0.955 | 1.000 | +4.7% WORSE |
| llvm fib | max_rss_kb | 0.627 | 0.623 | -0.6% |
| llvm sieve | wall_ms | 0.924 | 0.946 | +2.4% |
| llvm sieve | cpu_ms | 0.625 | 0.588 | -5.9% |
| llvm sieve | max_rss_kb | 0.413 | 0.402 | -2.7% |
| llvm nbody | wall_ms | 1.040 | 1.059 | +1.8% |
| llvm nbody | cpu_ms | 1.067 | 1.067 | +0.0% |
| llvm nbody | max_rss_kb | 0.621 | 0.614 | -1.1% |
| llvm sort | wall_ms | 0.815 | 0.805 | -1.2% |
| llvm sort | cpu_ms | 0.778 | 0.778 | +0.0% |
| llvm sort | max_rss_kb | 0.455 | 0.450 | -1.1% |
| llvm interp | wall_ms | 0.291 | 0.321 | +10.3% WORSE |
| llvm interp | cpu_ms | 0.250 | 0.286 | +14.4% WORSE |
| llvm interp | max_rss_kb | 0.623 | 0.621 | -0.3% |
| llvm collatz | wall_ms | 0.965 | 0.898 | -6.9% |
| llvm collatz | cpu_ms | 0.959 | 0.896 | -6.6% |
| llvm collatz | max_rss_kb | 0.624 | 0.616 | -1.3% |
| llvm strings | wall_ms | 0.458 | 0.460 | +0.4% |
| llvm strings | cpu_ms | 0.273 | 0.250 | -8.4% |
| llvm strings | max_rss_kb | 0.613 | 0.622 | +1.5% |
| coll putget | wall_ms | 0.579 | 0.595 | +2.8% |
| coll putget | cpu_ms | 0.500 | 0.500 | +0.0% |
| coll putget | max_rss_kb | 0.833 | 0.840 | +0.8% |
| coll get | wall_ms | 0.599 | 0.618 | +3.2% WORSE |
| coll get | cpu_ms | 1.000 | 0.500 | -50.0% |
| coll get | max_rss_kb | 0.971 | 0.976 | +0.5% |
| coll push | wall_ms | 0.735 | 0.731 | -0.5% |
| coll push | cpu_ms | 0.000 | 0.000 | n/a |
| coll push | max_rss_kb | 0.877 | 0.728 | -17.0% |
| coll update | wall_ms | 0.601 | 0.616 | +2.5% |
| coll update | cpu_ms | 0.500 | 0.500 | +0.0% |
| coll update | max_rss_kb | 0.833 | 0.831 | -0.2% |
| coll iter | wall_ms | 0.264 | 0.274 | +3.8% WORSE |
| coll iter | cpu_ms | 0.000 | 0.000 | n/a |
| coll iter | max_rss_kb | 0.975 | 0.980 | +0.5% |
| coll copymut | wall_ms | 0.675 | 0.632 | -6.4% |
| coll copymut | cpu_ms | 0.000 | 0.000 | n/a |
| coll copymut | max_rss_kb | 0.684 | 0.687 | +0.4% |
| coll branch | wall_ms | 0.025 | 0.025 | +0.0% |
| coll branch | cpu_ms | 0.000 | 0.000 | n/a |
| coll branch | max_rss_kb | 0.456 | 0.438 | -3.9% |
| conc spawn | wall_ms | 0.428 | 0.426 | -0.5% |
| conc spawn | cpu_ms | 0.133 | 0.133 | +0.0% |
| conc spawn | max_rss_kb | 0.504 | 0.492 | -2.4% |
| conc fanout | wall_ms | 0.667 | 0.687 | +3.0% |
| conc fanout | cpu_ms | 0.200 | 0.200 | +0.0% |
| conc fanout | max_rss_kb | 0.409 | 0.405 | -1.0% |
| conc pingpong | wall_ms | 0.897 | 0.873 | -2.7% |
| conc pingpong | cpu_ms | 1.000 | 1.000 | +0.0% |
| conc pingpong | max_rss_kb | 0.638 | 0.652 | +2.2% |
| conc mutex | wall_ms | 0.271 | 0.279 | +3.0% |
| conc mutex | cpu_ms | 0.000 | 0.000 | n/a |
| conc mutex | max_rss_kb | 0.555 | 0.567 | +2.2% |
| conc rwmutex | wall_ms | 0.897 | 0.901 | +0.4% |
| conc rwmutex | cpu_ms | 1.000 | 1.000 | +0.0% |
| conc rwmutex | max_rss_kb | 0.554 | 0.554 | +0.0% |
| conc pmap | wall_ms | 1.010 | 0.988 | -2.2% |
| conc pmap | cpu_ms | 1.018 | 0.982 | -3.5% |
| conc pmap | max_rss_kb | 0.794 | 0.796 | +0.3% |

---

# Xo against Go: xo 0.35.0 darwin/arm64 (2026-10-30)

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
| llvm | go | 1520 | 40 | 40 | 100 | 324 / 25 / 69 | 2.39 | - |
| coll | go | 1450 | 30 | 30 | 110 | 306 / 26 / 68 | 2.43 | - |
| conc | go | 1430 | 30 | 30 | 100 | 298 / 26 / 68 | 2.40 | - |
| llvm-fib | xo | 940 | 70 | 60 | 60 | 382 / 130 / 134 | 1.25 | separate, std records |
| llvm-sieve | xo | 880 | 70 | 60 | 60 | 380 / 129 / 137 | 1.25 | separate, std records |
| llvm-nbody | xo | 900 | 70 | 60 | 70 | 378 / 133 / 137 | 1.26 | separate, std records |
| llvm-sort | xo | 890 | 70 | 60 | 60 | 383 / 131 / 134 | 1.26 | separate, std records |
| llvm-interp | xo | 880 | 70 | 60 | 60 | 388 / 132 / 138 | 1.26 | separate, std records |
| llvm-collatz | xo | 890 | 70 | 60 | 60 | 382 / 131 / 132 | 1.25 | separate, std records |
| llvm-strings | xo | 900 | 70 | 60 | 60 | 380 / 129 / 135 | 1.25 | separate, std records |
| coll | xo | 930 | 140 | 120 | 130 | 395 / 147 / 153 | 1.36 | separate, std records |
| conc-spawn | xo | 1050 | 70 | 70 | 70 | 383 / 133 / 135 | 1.34 | separate, std records |
| conc-fanout | xo | 1060 | 70 | 60 | 60 | 388 / 131 / 135 | 1.34 | separate, std records |
| conc-pingpong | xo | 1060 | 70 | 60 | 60 | 385 / 136 / 136 | 1.36 | separate, std records |
| conc-mutex | xo | 1040 | 70 | 60 | 60 | 383 / 135 / 135 | 1.40 | separate, std records |
| conc-rwmutex | xo | 1100 | 70 | 60 | 60 | 385 / 134 / 139 | 1.40 | separate, std records |
| conc-pmap | xo | 1050 | 70 | 60 | 60 | 383 / 131 / 139 | 1.34 | separate, std records |

The Go column builds one binary per group (llvm, coll, conc); the Xo
programs are one per workload except coll.

## Run (xo bench --ab, A = Go, B = Xo, 10 rounds)

### llvm fib (load 5 at start)

```
A: llvm.go fib 
B: llvm-fib.xo fib 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.71/5.71 5.71/5.71 5.71/5.71 5.71/5.71 5.33/5.71 5.33/5.33 5.33/5.33 5.33/5.33 5.33/5.33 5.33/5.33

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               233.2       225.0       2.219% 4.392%     0.965   0.944 to 0.993    better (n=10)
cpu_ms                220.0       220.0       3.409% 4.545%     1.0     0.935 to 1.0      within noise (n=10)
max_rss_kb            4184        2608        2.103% 0.46%      0.623   0.617 to 0.634    better (n=10)
load: median 5.33 (A) / 5.52 (B); loaded: 0 of 10 rounds (at or above 8.71; baseline 5.71 + 3.0, at least 6.0)
```

### llvm sieve (load 5 at start)

```
A: llvm.go sieve 
B: llvm-sieve.xo sieve 
10 of 10 rounds used (0 loaded), loads A/B per round: 5.33/5.33 5.33/5.33 5.33/5.33 5.33/5.33 5.33/5.33 5.33/5.33 5.33/5.33 5.33/5.33 5.33/5.33 5.33/5.33

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               112.9       106.8       1.355% 5.49%      0.946   0.928 to 0.997    better (n=10)
cpu_ms                170.0       100.0       5.882% 0%         0.588   0.571 to 0.656    better (n=10)
max_rss_kb            45040       18128       7.647% 0%         0.402   0.385 to 0.42     better (n=10)
load: median 5.33 (A) / 5.33 (B); loaded: 0 of 10 rounds (at or above 8.33; baseline 5.33 + 3.0, at least 6.0)
```

### llvm nbody (load 4 at start)

```
A: llvm.go nbody 
B: llvm-nbody.xo nbody 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.98/4.98 4.98/4.98 4.98/4.98 4.98/4.98 4.98/4.98 4.98/4.98 4.98/4.98 4.98/4.98 4.98/4.98 4.98/4.98

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               162.8       172.3       0.703% 0.357%     1.059   1.049 to 1.062    worse (n=10)
cpu_ms                150.0       160.0       0% 0%             1.067   1.067 to 1.067    worse (n=10)
max_rss_kb            4264        2616        2.251% 1.835%     0.614   0.61 to 0.636     better (n=10)
load: median 4.98 (A) / 4.98 (B); loaded: 0 of 10 rounds (at or above 7.98; baseline 4.98 + 3.0, at least 6.0)
```

### llvm sort (load 4 at start)

```
A: llvm.go sort 
B: llvm-sort.xo sort 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.98/4.98 4.98/4.82 4.82/4.82 4.82/4.82 4.82/4.82 4.82/4.82 4.82/4.82 4.82/4.82 4.82/4.82 4.82/4.82

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               268.6       216.3       0.655% 0.598%     0.805   0.8 to 0.809      better (n=10)
cpu_ms                270.0       210.0       0% 0%             0.778   0.778 to 0.778    better (n=10)
max_rss_kb            76832       34608       8.903% 0%         0.45    0.41 to 0.456     better (n=10)
load: median 4.82 (A) / 4.82 (B); loaded: 0 of 10 rounds (at or above 7.98; baseline 4.98 + 3.0, at least 6.0)
```

### llvm interp (load 4 at start)

```
A: llvm.go interp 
B: llvm-interp.xo interp 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.82/4.82 4.82/4.82 4.82/4.82 4.52/4.52 4.52/4.52 4.52/4.52 4.52/4.52 4.52/4.52 4.52/4.52 4.52/4.52

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               146.5       47.06       0.939% 18.06%     0.321   0.288 to 0.349    better (n=10)
cpu_ms                140.0       40.0        0% 18.75%         0.286   0.214 to 0.286    better (n=10)
max_rss_kb            4200        2608        2.286% 0%         0.621   0.61 to 0.625     better (n=10)
load: median 4.52 (A) / 4.52 (B); loaded: 0 of 10 rounds (at or above 7.82; baseline 4.82 + 3.0, at least 6.0)
```

### llvm collatz (load 4 at start)

```
A: llvm.go collatz 
B: llvm-collatz.xo collatz 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.52/4.52 4.52/4.52 4.52/4.52 4.31/4.31 4.31/4.31 4.31/4.31 4.31/4.31 4.31/4.31 4.13/4.31 4.13/4.13

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               490.6       440.8       0.704% 0.433%     0.898   0.894 to 0.902    better (n=10)
cpu_ms                480.0       430.0       0% 0%             0.896   0.896 to 0.896    better (n=10)
max_rss_kb            4232        2608        1.134% 0.46%      0.616   0.61 to 0.619     better (n=10)
load: median 4.31 (A) / 4.31 (B); loaded: 0 of 10 rounds (at or above 7.52; baseline 4.52 + 3.0, at least 6.0)
```

### llvm strings (load 4 at start)

```
A: llvm.go strings 
B: llvm-strings.xo strings 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               91.6        42.12       0.956% 2.297%     0.46    0.455 to 0.47     better (n=10)
cpu_ms                120.0       30.0        6.25% 0%          0.25    0.25 to 0.273     better (n=10)
max_rss_kb            24576       15288       11.56% 0.105%     0.622   0.586 to 0.68     better (n=10)
load: median 4.13 (A) / 4.13 (B); loaded: 0 of 10 rounds (at or above 7.13; baseline 4.13 + 3.0, at least 6.0)
```

### coll putget (load 4 at start)

```
A: coll.go putget 
B: coll.xo putget 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               50.38       29.98       2.59% 2.617%      0.595   0.583 to 0.612    better (n=10)
cpu_ms                40.0        20.0        0% 0%             0.5     0.5 to 0.5        too coarse (n=10)
max_rss_kb            8784        7376        1.594% 0.434%     0.84    0.832 to 0.851    better (n=10)
load: median 4.13 (A) / 4.13 (B); loaded: 0 of 10 rounds (at or above 7.13; baseline 4.13 + 3.0, at least 6.0)
```

### coll get (load 4 at start)

```
A: coll.go get 
B: coll.xo get 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               23.12       14.28       5.139% 1.927%     0.618   0.594 to 0.628    better (n=10)
cpu_ms                20.0        10.0        37.5% 0%          0.5     0.5 to 1.0        too coarse (n=10)
max_rss_kb            7096        6928        0.564% 0.577%     0.976   0.973 to 0.985    better (n=10)
load: median 4.13 (A) / 4.13 (B); loaded: 0 of 10 rounds (at or above 7.13; baseline 4.13 + 3.0, at least 6.0)
```

### coll push (load 4 at start)

```
A: coll.go push 
B: coll.xo push 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13 4.13/4.13

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               12.34       9.015       4.785% 1.442%     0.731   0.704 to 0.75     better (n=10)
cpu_ms                10.0        0           75.0% 0%          0       0 to 0            too coarse (n=10)
max_rss_kb            70048       50960       23.01% 0.031%     0.728   0.722 to 0.938    better (n=10)
load: median 4.13 (A) / 4.13 (B); loaded: 0 of 10 rounds (at or above 7.13; baseline 4.13 + 3.0, at least 6.0)
```

### coll update (load 4 at start)

```
A: coll.go update 
B: coll.xo update 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.2/4.13 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               31.42       19.34       1.894% 2.533%     0.616   0.606 to 0.625    better (n=10)
cpu_ms                20.0        10.0        0% 0%             0.5     0.5 to 0.5        too coarse (n=10)
max_rss_kb            8848        7352        1.899% 0.435%     0.831   0.818 to 0.842    better (n=10)
load: median 4.2 (A) / 4.2 (B); loaded: 0 of 10 rounds (at or above 7.13; baseline 4.13 + 3.0, at least 6.0)
```

### coll iter (load 4 at start)

```
A: coll.go iter 
B: coll.xo iter 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.47       6.161       1.991% 4.172%     0.274   0.27 to 0.284     better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            7104        6960        1.07% 0.46%       0.98    0.969 to 0.984    better (n=10)
load: median 4.2 (A) / 4.2 (B); loaded: 0 of 10 rounds (at or above 7.2; baseline 4.2 + 3.0, at least 6.0)
```

### coll copymut (load 4 at start)

```
A: coll.go copymut 
B: coll.xo copymut 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               14.39       9.098       1.072% 2.904%     0.632   0.627 to 0.651    better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            10128       6960        5.687% 0.23%      0.687   0.657 to 0.699    better (n=10)
load: median 4.2 (A) / 4.2 (B); loaded: 0 of 10 rounds (at or above 7.2; baseline 4.2 + 3.0, at least 6.0)
```

### coll branch (load 4 at start)

```
A: coll.go branch 
B: coll.xo branch 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               202.2       5.089       4.122% 4.77%      0.025   0.024 to 0.026    better (n=10)
cpu_ms                310.0       0           3.226% 0%         0       0 to 0            better (n=10)
max_rss_kb            15856       6944        8.956% 0%         0.438   0.419 to 0.47     better (n=10)
load: median 4.2 (A) / 4.2 (B); loaded: 0 of 10 rounds (at or above 7.2; baseline 4.2 + 3.0, at least 6.0)
```

### conc spawn (load 4 at start)

```
A: conc.go spawn 
B: conc-spawn.xo spawn 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               50.93       21.71       5.476% 4.271%     0.426   0.407 to 0.433    better (n=10)
cpu_ms                150.0       20.0        11.67% 37.5%      0.133   0.067 to 0.133    better (n=10)
max_rss_kb            11224       5520        2.174% 11.23%     0.492   0.461 to 0.53     better (n=10)
load: median 4.2 (A) / 4.2 (B); loaded: 0 of 10 rounds (at or above 7.2; baseline 4.2 + 3.0, at least 6.0)
```

### conc fanout (load 4 at start)

```
A: conc.go fanout 
B: conc-fanout.xo fanout 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2 4.2/4.2

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.05       15.15       9.026% 2.511%     0.687   0.634 to 0.715    better (n=10)
cpu_ms                50.0        10.0        0% 0%             0.2     0.182 to 0.2      too coarse (n=10)
max_rss_kb            11728       4744        5.832% 5.396%     0.405   0.382 to 0.426    better (n=10)
load: median 4.2 (A) / 4.2 (B); loaded: 0 of 10 rounds (at or above 7.2; baseline 4.2 + 3.0, at least 6.0)
```

### conc pingpong (load 4 at start)

```
A: conc.go pingpong 
B: conc-pingpong.xo pingpong 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.2/4.2 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               20.03       17.48       1.785% 1.363%     0.873   0.859 to 0.881    better (n=10)
cpu_ms                10.0        10.0        0% 0%             1.0     1.0 to 1.0        too coarse (n=10)
max_rss_kb            4248        2768        1.601% 1.59%      0.652   0.642 to 0.659    better (n=10)
load: median 4.1 (A) / 4.1 (B); loaded: 0 of 10 rounds (at or above 7.2; baseline 4.2 + 3.0, at least 6.0)
```

### conc mutex (load 4 at start)

```
A: conc.go mutex 
B: conc-mutex.xo mutex 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.79       6.348       3.502% 1.028%     0.279   0.27 to 0.287     better (n=10)
cpu_ms                90.0        0           8.333% 0%         0       0 to 0            too coarse (n=10)
max_rss_kb            4768        2704        1.846% 1.775%     0.567   0.556 to 0.574    better (n=10)
load: median 4.1 (A) / 4.1 (B); loaded: 0 of 10 rounds (at or above 7.1; baseline 4.1 + 3.0, at least 6.0)
```

### conc rwmutex (load 4 at start)

```
A: conc.go rwmutex 
B: conc-rwmutex.xo rwmutex 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               9.621       8.667       1.564% 2.986%     0.901   0.874 to 0.909    better (n=10)
cpu_ms                0           0           0% 0%             1.0     1.0 to 1.0        too coarse (n=10)
max_rss_kb            4880        2704        2.459% 1.036%     0.554   0.55 to 0.568     better (n=10)
load: median 4.1 (A) / 4.1 (B); loaded: 0 of 10 rounds (at or above 7.1; baseline 4.1 + 3.0, at least 6.0)
```

### conc pmap (load 4 at start)

```
A: conc.go pmap 
B: conc-pmap.xo pmap 
10 of 10 rounds used (0 loaded), loads A/B per round: 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1 4.1/4.1

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               56.3        55.62       1.924% 1.77%      0.988   0.971 to 1.005    within noise (n=10)
cpu_ms                550.0       540.0       1.818% 0%         0.982   0.982 to 1.0      within noise (n=10)
max_rss_kb            6832        5440        1.112% 0.882%     0.796   0.787 to 0.801    better (n=10)
load: median 4.1 (A) / 4.1 (B); loaded: 0 of 10 rounds (at or above 7.1; baseline 4.1 + 3.0, at least 6.0)
```

