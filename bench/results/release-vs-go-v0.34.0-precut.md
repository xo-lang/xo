# Xo against Go: main before the 0.34.0 cut (2026-10-29)

A pre-cut run of scripts/release-vs-go.sh (the lead, 2026-10-29: regressions
should show before the cut, not after) on main c462dd4e1 with the worktree's xo
(decision 0388 separate compilation and its std records on by default,
decision 0398 jump padding on x86 irrelevant here), Darwin arm64, 24 GB, Go go1.26.1.
The report below is the script's; the comparison table against
release-vs-go-v0.33.0.md follows it, each run row's B/A ratio (Xo over Go) old
against new, a row worse by more than 3% flagged.

## Against 0.33.0

Seven of 60 ratios are worse than 0.33.0's by more than 3%, 0 of them past
Go: llvm sieve wall 0.926 to 0.996 and cpu 0.606 to 0.656 (the one row worth
a direct A/B of the 0.33.0 xo against main's before the cut); llvm nbody cpu
1.032 to 1.067 (nbody was already the slowest row, decision 0384's float
work); llvm sort peak RSS 0.417 to 0.454; llvm strings cpu 0.25 to 0.273;
coll putget wall 0.581 to 0.599 and coll get 0.602 to 0.627. Better by more
than 3%: interp 0.500 to 0.296, pingpong 1.196 to 0.884, pmap 1.086 to
0.993, spawn 0.463 to 0.424, collatz 0.960 to 0.919, mutex, iter, copymut,
push. Every run started at load 2 or 3. Compile: xo's edit builds 180 to 210
ms against 0.33.0's 210 to 230 (separate compilation on by default; whether
the edit build already had std's variants, built in the background after
the cold build, the report does not say: gotchas/sepcomp-default-check-
needs-std-variants.md), cold 890 to 1050 against 920 to 1090.

| workload | metric | 0.33.0 B/A | pre-cut B/A | change |
|---|---|---|---|---|
| llvm fib | wall_ms | 0.957 | 0.958 | +0.1% |
| llvm fib | cpu_ms | 0.955 | 0.955 | +0.0% |
| llvm fib | max_rss_kb | 0.612 | 0.612 | +0.0% |
| llvm sieve | wall_ms | 0.926 | 0.996 | +7.6% WORSE |
| llvm sieve | cpu_ms | 0.606 | 0.656 | +8.3% WORSE |
| llvm sieve | max_rss_kb | 0.392 | 0.389 | -0.8% |
| llvm nbody | wall_ms | 1.054 | 1.058 | +0.4% |
| llvm nbody | cpu_ms | 1.032 | 1.067 | +3.4% WORSE |
| llvm nbody | max_rss_kb | 0.611 | 0.607 | -0.7% |
| llvm sort | wall_ms | 0.806 | 0.806 | +0.0% |
| llvm sort | cpu_ms | 0.778 | 0.778 | +0.0% |
| llvm sort | max_rss_kb | 0.417 | 0.454 | +8.9% WORSE |
| llvm interp | wall_ms | 0.500 | 0.296 | -40.8% |
| llvm interp | cpu_ms | 0.429 | 0.286 | -33.3% |
| llvm interp | max_rss_kb | 0.606 | 0.615 | +1.5% |
| llvm collatz | wall_ms | 0.960 | 0.919 | -4.3% |
| llvm collatz | cpu_ms | 0.957 | 0.908 | -5.1% |
| llvm collatz | max_rss_kb | 0.612 | 0.610 | -0.3% |
| llvm strings | wall_ms | 0.462 | 0.454 | -1.7% |
| llvm strings | cpu_ms | 0.250 | 0.273 | +9.2% WORSE |
| llvm strings | max_rss_kb | 0.644 | 0.623 | -3.3% |
| coll putget | wall_ms | 0.581 | 0.599 | +3.1% WORSE |
| coll putget | cpu_ms | 0.500 | 0.500 | +0.0% |
| coll putget | max_rss_kb | 0.832 | 0.832 | +0.0% |
| coll get | wall_ms | 0.602 | 0.627 | +4.2% WORSE |
| coll get | cpu_ms | 0.500 | 0.500 | +0.0% |
| coll get | max_rss_kb | 0.969 | 0.964 | -0.5% |
| coll push | wall_ms | 0.743 | 0.720 | -3.1% |
| coll push | cpu_ms | 0.000 | 0.000 | +0.0% |
| coll push | max_rss_kb | 0.932 | 0.803 | -13.8% |
| coll update | wall_ms | 0.613 | 0.609 | -0.7% |
| coll update | cpu_ms | 0.500 | 0.500 | +0.0% |
| coll update | max_rss_kb | 0.835 | 0.827 | -1.0% |
| coll iter | wall_ms | 0.277 | 0.268 | -3.2% |
| coll iter | cpu_ms | 0.000 | 0.000 | +0.0% |
| coll iter | max_rss_kb | 0.967 | 0.973 | +0.6% |
| coll copymut | wall_ms | 0.661 | 0.639 | -3.3% |
| coll copymut | cpu_ms | 0.000 | 0.000 | +0.0% |
| coll copymut | max_rss_kb | 0.687 | 0.671 | -2.3% |
| coll branch | wall_ms | 0.025 | 0.025 | +0.0% |
| coll branch | cpu_ms | 0.000 | 0.000 | +0.0% |
| coll branch | max_rss_kb | 0.452 | 0.458 | +1.3% |
| conc spawn | wall_ms | 0.463 | 0.424 | -8.4% |
| conc spawn | cpu_ms | 0.133 | 0.067 | -49.6% |
| conc spawn | max_rss_kb | 0.535 | 0.475 | -11.2% |
| conc fanout | wall_ms | 0.704 | 0.713 | +1.3% |
| conc fanout | cpu_ms | 0.200 | 0.200 | +0.0% |
| conc fanout | max_rss_kb | 0.376 | 0.383 | +1.9% |
| conc pingpong | wall_ms | 1.196 | 0.884 | -26.1% |
| conc pingpong | cpu_ms | 2.000 | 1.000 | -50.0% |
| conc pingpong | max_rss_kb | 0.626 | 0.633 | +1.1% |
| conc mutex | wall_ms | 0.284 | 0.273 | -3.9% |
| conc mutex | cpu_ms | 0.000 | 0.000 | +0.0% |
| conc mutex | max_rss_kb | 0.542 | 0.551 | +1.7% |
| conc rwmutex | wall_ms | 0.869 | 0.892 | +2.6% |
| conc rwmutex | cpu_ms | 0.000 | 1.000 | +0.0% |
| conc rwmutex | max_rss_kb | 0.541 | 0.548 | +1.3% |
| conc pmap | wall_ms | 1.086 | 0.993 | -8.6% |
| conc pmap | cpu_ms | 1.093 | 1.000 | -8.5% |
| conc pmap | max_rss_kb | 0.789 | 0.789 | +0.0% |

flagged (worse than 0.33.0 by more than 3%): 7
   ('llvm sieve', 'wall_ms', 0.926, 0.996)
   ('llvm sieve', 'cpu_ms', 0.606, 0.656)
   ('llvm nbody', 'cpu_ms', 1.032, 1.067)
   ('llvm sort', 'max_rss_kb', 0.417, 0.454)
   ('llvm strings', 'cpu_ms', 0.25, 0.273)
   ('coll putget', 'wall_ms', 0.581, 0.599)
   ('coll get', 'wall_ms', 0.602, 0.627)


## The flagged sieve row, directly (2026-10-29)

The 0.33.0 release xo's sieve against main c462dd4e1's, `xo bench --ab`,
20 rounds, load 2.5, nothing else running: wall 109.0 against 105.7 ms,
B/A 0.969 (0.92 to 1.055), within noise; cpu 100 against 100; sieve's
round spread is 9 to 13%. The report's 0.926 to 0.996 against Go was two
single runs' noise, not a regression. The other six flagged ratios are
within their rows' spreads too (nbody cpu 3%, strings cpu 2 ms, coll
putget and get 2 to 4%); llvm sort's peak RSS 0.417 to 0.454 is the one
left unexplained (18.1 MB either way for sieve; sort's own RSS figures are
in the run below).

## The run (Xo against Go: xo 0.33.0 darwin/arm64 (2026-10-29))

scripts/release-vs-go.sh on Darwin arm64, 24 GB,
Go go1.26.1. Build times are wall milliseconds of one build; peak MB is the
build's largest resident set (cold / warm / edit). Warm is the third unchanged build
(xo's parts pack is written on the second and read from the third); second is the second.
Size is the default build's binary.

## Compile and size

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB |
|---|---|---|---|---|---|---|---|
| llvm | go | 1440 | 40 | 40 | 100 | 304 / 26 / 67 | 2.39 |
| coll | go | 1420 | 40 | 40 | 110 | 297 / 26 / 67 | 2.43 |
| conc | go | 1410 | 40 | 40 | 100 | 295 / 25 / 68 | 2.40 |
| llvm-fib | xo | 910 | 10 | 10 | 180 | 411 / 14 / 217 | 1.66 |
| llvm-sieve | xo | 890 | 10 | 10 | 190 | 419 / 14 / 217 | 1.67 |
| llvm-nbody | xo | 890 | 10 | 10 | 190 | 421 / 14 / 219 | 1.67 |
| llvm-sort | xo | 890 | 10 | 10 | 210 | 418 / 14 / 218 | 1.67 |
| llvm-interp | xo | 950 | 10 | 10 | 190 | 420 / 14 / 216 | 1.67 |
| llvm-collatz | xo | 890 | 10 | 10 | 190 | 416 / 14 / 219 | 1.67 |
| llvm-strings | xo | 900 | 20 | 10 | 180 | 416 / 14 / 219 | 1.67 |
| coll | xo | 930 | 10 | 10 | 200 | 426 / 14 / 225 | 1.76 |
| conc-spawn | xo | 1040 | 10 | 10 | 180 | 418 / 14 / 217 | 1.69 |
| conc-fanout | xo | 1050 | 10 | 10 | 180 | 424 / 14 / 217 | 1.69 |
| conc-pingpong | xo | 1050 | 10 | 10 | 190 | 421 / 14 / 218 | 1.69 |
| conc-mutex | xo | 1040 | 10 | 10 | 180 | 411 / 14 / 218 | 1.69 |
| conc-rwmutex | xo | 1050 | 10 | 10 | 190 | 417 / 14 / 220 | 1.71 |
| conc-pmap | xo | 1040 | 10 | 10 | 180 | 416 / 14 / 218 | 1.69 |

The Go column builds one binary per group (llvm, coll, conc); the Xo
programs are one per workload except coll.

## Run (xo bench --ab, A = Go, B = Xo, 10 rounds)

### llvm fib (load 3 at start)

```
A: llvm.go fib 
B: llvm-fib.xo fib 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.73/3.73 3.73/3.73 3.73/3.73 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               230.8       221.2       4.895% 1.42%      0.958   0.918 to 0.979    better (n=10)
cpu_ms                220.0       210.0       3.409% 0%         0.955   0.913 to 0.956    better (n=10)
max_rss_kb            4224        2584        3.598% 1.858%     0.612   0.596 to 0.632    better (n=10)
load: median 3.51 (A) / 3.51 (B); loaded: 0 of 10 rounds (at or above 6.73; baseline 3.73 + 3.0, at least 6.0)
```

### llvm sieve (load 3 at start)

```
A: llvm.go sieve 
B: llvm-sieve.xo sieve 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.51/3.51 3.31/3.51 3.31/3.31 3.31/3.31 3.31/3.31

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               111.8       111.3       1.596% 12.54%     0.996   0.922 to 1.052    within noise (n=10)
cpu_ms                160.0       105.0       15.62% 16.67%     0.656   0.556 to 0.71     better (n=10)
max_rss_kb            46568       18096       8.143% 0%         0.389   0.38 to 0.422     better (n=10)
load: median 3.51 (A) / 3.51 (B); loaded: 0 of 10 rounds (at or above 6.51; baseline 3.51 + 3.0, at least 6.0)
```

### llvm nbody (load 3 at start)

```
A: llvm.go nbody 
B: llvm-nbody.xo nbody 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.31/3.31 3.31/3.31 3.31/3.31 3.31/3.31 3.31/3.31 3.31/3.31 3.31/3.31 3.31/3.31 3.31/3.31 3.31/3.31

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               162.1       171.6       0.334% 0.42%      1.058   1.054 to 1.066    worse (n=10)
cpu_ms                150.0       160.0       0% 0%             1.067   1.067 to 1.1      worse (n=10)
max_rss_kb            4272        2592        1.498% 1.08%      0.607   0.6 to 0.618      better (n=10)
load: median 3.31 (A) / 3.31 (B); loaded: 0 of 10 rounds (at or above 6.31; baseline 3.31 + 3.0, at least 6.0)
```

### llvm sort (load 3 at start)

```
A: llvm.go sort 
B: llvm-sort.xo sort 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.2/3.2 3.2/3.2 3.2/3.2 3.2/3.2 3.2/3.2 3.2/3.2 3.2/3.2 3.2/3.2 3.2/3.2 3.2/3.2

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               266.6       215.0       0.688% 0.104%     0.806   0.803 to 0.809    better (n=10)
cpu_ms                270.0       210.0       0% 0%             0.778   0.778 to 0.778    better (n=10)
max_rss_kb            76232       34592       12.82% 0%         0.454   0.402 to 0.497    better (n=10)
load: median 3.2 (A) / 3.2 (B); loaded: 0 of 10 rounds (at or above 6.31; baseline 3.31 + 3.0, at least 6.0)
```

### llvm interp (load 3 at start)

```
A: llvm.go interp 
B: llvm-interp.xo interp 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.03/3.03 3.03/3.03 3.03/3.03 3.03/3.03 3.03/3.03 3.03/3.03 3.03/3.03 3.03/3.03 3.03/3.03 3.03/3.03

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               145.5       43.06       2.022% 8.786%     0.296   0.274 to 0.311    better (n=10)
cpu_ms                140.0       40.0        0% 25.0%          0.286   0.214 to 0.296    better (n=10)
max_rss_kb            4216        2592        2.277% 0%         0.615   0.607 to 0.621    better (n=10)
load: median 3.03 (A) / 3.03 (B); loaded: 0 of 10 rounds (at or above 6.2; baseline 3.2 + 3.0, at least 6.0)
```

### llvm collatz (load 3 at start)

```
A: llvm.go collatz 
B: llvm-collatz.xo collatz 
10 of 10 rounds used (0 loaded), loads A/B per round: 3.03/3.03 3.03/3.03 2.86/3.03 2.86/2.86 2.86/2.86 2.86/2.86 2.86/2.86 2.86/2.8 2.8/2.8 2.8/2.8

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               493.5       453.3       0.673% 0.417%     0.919   0.912 to 0.922    better (n=10)
cpu_ms                490.0       445.0       2.041% 2.247%     0.908   0.898 to 0.938    better (n=10)
max_rss_kb            4224        2576        1.989% 0%         0.61    0.605 to 0.619    better (n=10)
load: median 2.86 (A) / 2.86 (B); loaded: 0 of 10 rounds (at or above 6.03; baseline 3.03 + 3.0, at least 6.0)
```

### llvm strings (load 2 at start)

```
A: llvm.go strings 
B: llvm-strings.xo strings 
10 of 10 rounds used (0 loaded), loads A/B per round: 2.8/2.8 2.8/2.8 2.8/2.8 2.8/2.8 2.8/2.8 2.8/2.8 2.8/2.8 2.8/2.8 2.8/2.8 2.8/2.8

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               90.55       41.13       2.276% 1.941%     0.454   0.446 to 0.464    better (n=10)
cpu_ms                110.0       30.0        0% 0%             0.273   0.261 to 0.273    better (n=10)
max_rss_kb            24472       15248       6.979% 0.105%     0.623   0.601 to 0.664    better (n=10)
load: median 2.8 (A) / 2.8 (B); loaded: 0 of 10 rounds (at or above 6.0; baseline 2.8 + 3.0, at least 6.0)
```

### coll putget (load 2 at start)

```
A: coll.go putget 
B: coll.xo putget 
10 of 10 rounds used (0 loaded), loads A/B per round: 2.8/2.8 2.8/2.8 2.8/2.8 2.8/2.8 2.8/2.8 2.8/2.8 2.8/2.8 2.8/2.8 2.8/2.8 2.8/2.8

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               51.78       31.03       1.833% 2.9%       0.599   0.592 to 0.613    better (n=10)
cpu_ms                40.0        20.0        0% 0%             0.5     0.5 to 0.5        too coarse (n=10)
max_rss_kb            8784        7312        1.594% 0.602%     0.832   0.825 to 0.845    better (n=10)
load: median 2.8 (A) / 2.8 (B); loaded: 0 of 10 rounds (at or above 6.0; baseline 2.8 + 3.0, at least 6.0)
```

### coll get (load 2 at start)

```
A: coll.go get 
B: coll.xo get 
10 of 10 rounds used (0 loaded), loads A/B per round: 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.84       14.32       1.367% 2.589%     0.627   0.618 to 0.647    better (n=10)
cpu_ms                20.0        10.0        50.0% 0%          0.5     0.5 to 1.0        too coarse (n=10)
max_rss_kb            7152        6896        1.51% 0.232%      0.964   0.956 to 0.971    better (n=10)
load: median 2.73 (A) / 2.73 (B); loaded: 0 of 10 rounds (at or above 6.0; baseline 2.8 + 3.0, at least 6.0)
```

### coll push (load 2 at start)

```
A: coll.go push 
B: coll.xo push 
10 of 10 rounds used (0 loaded), loads A/B per round: 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               12.29       8.848       4.648% 3.337%     0.72    0.702 to 0.751    better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            63440       50944       25.04% 0.079%     0.803   0.725 to 0.938    better (n=10)
load: median 2.73 (A) / 2.73 (B); loaded: 0 of 10 rounds (at or above 6.0; baseline 2.73 + 3.0, at least 6.0)
```

### coll update (load 2 at start)

```
A: coll.go update 
B: coll.xo update 
10 of 10 rounds used (0 loaded), loads A/B per round: 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               31.13       18.97       1.645% 3.751%     0.609   0.601 to 0.625    better (n=10)
cpu_ms                20.0        10.0        0% 0%             0.5     0.5 to 0.5        too coarse (n=10)
max_rss_kb            8840        7312        0.814% 0.383%     0.827   0.824 to 0.838    better (n=10)
load: median 2.73 (A) / 2.73 (B); loaded: 0 of 10 rounds (at or above 6.0; baseline 2.73 + 3.0, at least 6.0)
```

### coll iter (load 2 at start)

```
A: coll.go iter 
B: coll.xo iter 
10 of 10 rounds used (0 loaded), loads A/B per round: 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               21.91       5.864       0.899% 2.865%     0.268   0.262 to 0.274    better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            7088        6896        1.354% 0.174%     0.973   0.962 to 0.977    better (n=10)
load: median 2.73 (A) / 2.73 (B); loaded: 0 of 10 rounds (at or above 6.0; baseline 2.73 + 3.0, at least 6.0)
```

### coll copymut (load 2 at start)

```
A: coll.go copymut 
B: coll.xo copymut 
10 of 10 rounds used (0 loaded), loads A/B per round: 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               14.23       9.093       2.837% 1.696%     0.639   0.626 to 0.653    better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            10296       6912        3.302% 0.231%     0.671   0.661 to 0.689    better (n=10)
load: median 2.73 (A) / 2.73 (B); loaded: 0 of 10 rounds (at or above 6.0; baseline 2.73 + 3.0, at least 6.0)
```

### coll branch (load 2 at start)

```
A: coll.go branch 
B: coll.xo branch 
10 of 10 rounds used (0 loaded), loads A/B per round: 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               199.9       4.964       0.709% 3.611%     0.025   0.024 to 0.025    better (n=10)
cpu_ms                300.0       0           3.333% 0%         0       0 to 0            better (n=10)
max_rss_kb            15096       6912        6.068% 0%         0.458   0.425 to 0.474    better (n=10)
load: median 2.73 (A) / 2.73 (B); loaded: 0 of 10 rounds (at or above 6.0; baseline 2.73 + 3.0, at least 6.0)
```

### conc spawn (load 2 at start)

```
A: conc.go spawn 
B: conc-spawn.xo spawn 
10 of 10 rounds used (0 loaded), loads A/B per round: 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.73 2.73/2.83

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               49.49       21.0        3.535% 3.769%     0.424   0.421 to 0.436    better (n=10)
cpu_ms                150.0       10.0        0% 100.0%         0.067   0.067 to 0.133    better (n=10)
max_rss_kb            11072       5256        4.227% 18.26%     0.475   0.425 to 0.531    better (n=10)
load: median 2.73 (A) / 2.73 (B); loaded: 0 of 10 rounds (at or above 6.0; baseline 2.73 + 3.0, at least 6.0)
```

### conc fanout (load 2 at start)

```
A: conc.go fanout 
B: conc-fanout.xo fanout 
10 of 10 rounds used (0 loaded), loads A/B per round: 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               21.0        14.97       6.258% 1.913%     0.713   0.662 to 0.73     better (n=10)
cpu_ms                50.0        10.0        0% 0%             0.2     0.2 to 0.2        too coarse (n=10)
max_rss_kb            11936       4576        2.916% 2.098%     0.383   0.377 to 0.396    better (n=10)
load: median 2.83 (A) / 2.83 (B); loaded: 0 of 10 rounds (at or above 6.0; baseline 2.83 + 3.0, at least 6.0)
```

### conc pingpong (load 2 at start)

```
A: conc.go pingpong 
B: conc-pingpong.xo pingpong 
10 of 10 rounds used (0 loaded), loads A/B per round: 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               19.41       17.15       2.89% 1.707%      0.884   0.868 to 0.903    better (n=10)
cpu_ms                10.0        10.0        0% 0%             1.0     1.0 to 1.0        too coarse (n=10)
max_rss_kb            4256        2696        3.102% 1.039%     0.633   0.62 to 0.649     better (n=10)
load: median 2.83 (A) / 2.83 (B); loaded: 0 of 10 rounds (at or above 6.0; baseline 2.83 + 3.0, at least 6.0)
```

### conc mutex (load 2 at start)

```
A: conc.go mutex 
B: conc-mutex.xo mutex 
10 of 10 rounds used (0 loaded), loads A/B per round: 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.76       6.212       2.716% 1.002%     0.273   0.268 to 0.28     better (n=10)
cpu_ms                90.0        0           8.333% 0%         0       0 to 0            too coarse (n=10)
max_rss_kb            4760        2624        3.025% 1.524%     0.551   0.545 to 0.566    better (n=10)
load: median 2.83 (A) / 2.83 (B); loaded: 0 of 10 rounds (at or above 6.0; baseline 2.83 + 3.0, at least 6.0)
```

### conc rwmutex (load 2 at start)

```
A: conc.go rwmutex 
B: conc-rwmutex.xo rwmutex 
10 of 10 rounds used (0 loaded), loads A/B per round: 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               9.317       8.315       2.892% 1.819%     0.892   0.873 to 0.914    better (n=10)
cpu_ms                0           0           0% 0%             1.0     1.0 to 1.0        too coarse (n=10)
max_rss_kb            4832        2648        1.159% 1.511%     0.548   0.541 to 0.555    better (n=10)
load: median 2.83 (A) / 2.83 (B); loaded: 0 of 10 rounds (at or above 6.0; baseline 2.83 + 3.0, at least 6.0)
```

### conc pmap (load 2 at start)

```
A: conc.go pmap 
B: conc-pmap.xo pmap 
10 of 10 rounds used (0 loaded), loads A/B per round: 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83 2.83/2.83

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               54.41       54.02       0.972% 0.771%     0.993   0.984 to 1.0      better (n=10)
cpu_ms                550.0       550.0       0% 0%             1.0     1.0 to 1.0        within noise (n=10)
max_rss_kb            6832        5392        1.581% 0.297%     0.789   0.781 to 0.801    better (n=10)
load: median 2.83 (A) / 2.83 (B); loaded: 0 of 10 rounds (at or above 6.0; baseline 2.83 + 3.0, at least 6.0)
```

