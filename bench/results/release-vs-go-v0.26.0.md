# Xo against Go: xo 0.26.0 darwin/arm64 (2026-10-23)

Release step 7 (toolchain.md): the released darwin/arm64 archive of
v0.26.0 (public commit f5a2ec2e6, xo stamped with it), unpacked, against
Go 1.26.1, by scripts/release-vs-go.sh in a bulk slot on the development
Mac at load about 14 (other agents running): wall times are noisy,
`--ab`'s verdicts account for the spread of its 10 rounds.

Summary:

- Run time, Xo over Go (wall, median): Xo is faster on 14 of 20
  workloads (fib 0.98, collatz 0.95, interp 0.48, strings 0.44, sort 0.80, every collection
  workload 0.02 to 0.86, conc mutex 0.56, fanout 0.89), within noise on
  3 (sieve, spawn, pmap), and slower on 3: conc rwmutex 3.78, conc
  pingpong 1.51, llvm nbody 1.43. Those three are the next targets.
- Peak memory of the run: Xo lower on all 20 (0.38 to 0.97 of Go's).
- Binary size: 1.81 to 1.89 MB against Go's 2.39 to 2.43 MB.
- Compile: cold builds 2.0 to 2.4 s against Go's 1.9 to 2.0 s; warm
  builds 0.42 to 0.64 s against 0.04 to 0.08 s, and a one function edit
  0.39 to 0.46 s against 0.12 to 0.32 s; xo's build peaks at 0.76 to
  0.89 GB against Go's 0.29 to 0.31 GB cold (26 to 68 MB warm). A warm
  build of an unchanged program is xo's largest gap to Go.

scripts/release-vs-go.sh on Darwin arm64, 24 GB,
Go go1.26.1. Build times are wall milliseconds of one build; peak MB is the
build's largest resident set (cold / warm / edit). Size is the default build's binary.

## Compile and size

| program | side | cold ms | warm ms | edit ms | peak MB (c/w/e) | size MB |
|---|---|---|---|---|---|---|
| llvm | go | 1880 | 70 | 120 | 302 / 26 / 68 | 2.39 |
| coll | go | 1960 | 40 | 320 | 294 / 26 / 68 | 2.43 |
| conc | go | 1990 | 80 | 120 | 306 / 27 / 66 | 2.40 |
| llvm-fib | xo | 2160 | 460 | 400 | 877 / 780 / 725 | 1.81 |
| llvm-sieve | xo | 2310 | 640 | 450 | 878 / 782 / 728 | 1.81 |
| llvm-nbody | xo | 2040 | 510 | 420 | 882 / 785 / 732 | 1.82 |
| llvm-sort | xo | 2029 | 470 | 460 | 879 / 786 / 730 | 1.82 |
| llvm-interp | xo | 2130 | 420 | 390 | 881 / 782 / 719 | 1.82 |
| llvm-collatz | xo | 2240 | 470 | 400 | 763 / 786 / 725 | 1.81 |
| llvm-strings | xo | 2250 | 520 | 400 | 877 / 776 / 728 | 1.82 |
| coll | xo | 2130 | 460 | 450 | 888 / 806 / 739 | 1.89 |
| conc-spawn | xo | 2270 | 550 | 430 | 857 / 781 / 732 | 1.81 |
| conc-fanout | xo | 2410 | 470 | 430 | 872 / 783 / 729 | 1.81 |
| conc-pingpong | xo | 2210 | 470 | 440 | 873 / 787 / 722 | 1.81 |
| conc-mutex | xo | 2420 | 470 | 450 | 857 / 776 / 722 | 1.81 |
| conc-rwmutex | xo | 2270 | 440 | 430 | 869 / 777 / 730 | 1.81 |
| conc-pmap | xo | 2240 | 460 | 440 | 872 / 783 / 730 | 1.81 |

The Go column builds one binary per group (llvm, coll, conc); the Xo
programs are one per workload except coll.

## Run (xo bench --ab, A = Go, B = Xo, 10 rounds)

### llvm fib

```
A: llvm.go fib 
B: llvm-fib.xo fib 
10 of 10 rounds used (0 loaded), loads A/B per round: 14.33/14.33 14.33/14.33 14.33/14.33 14.33/14.33 14.33/14.33 14.33/14.33 14.33/14.33 14.33/14.33 14.33/14.33 14.33/14.33

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               238.2       232.8       1.723% 1.965%     0.977   0.959 to 0.992    better (n=10)
cpu_ms                230.0       220.0       3.261% 0%         0.957   0.957 to 1.0      within noise (n=10)
max_rss_kb            4152        2640        3.179% 1.212%     0.636   0.62 to 0.646     better (n=10)
```

### llvm sieve

```
A: llvm.go sieve 
B: llvm-sieve.xo sieve 
10 of 10 rounds used (0 loaded), loads A/B per round: 13.82/13.82 13.82/13.82 13.82/13.82 13.82/13.82 13.82/13.82 13.82/13.82 13.82/13.82 13.82/13.82 13.82/13.82 13.82/13.82

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               116.5       112.6       5.862% 7.504%     0.966   0.898 to 1.003    within noise (n=10)
cpu_ms                170.0       100.0       4.412% 7.5%       0.588   0.556 to 0.647    better (n=10)
max_rss_kb            45912       18152       6.282% 0.154%     0.395   0.381 to 0.417    better (n=10)
```

### llvm nbody

```
A: llvm.go nbody 
B: llvm-nbody.xo nbody 
10 of 10 rounds used (0 loaded), loads A/B per round: 13.82/13.82 13.82/13.82 13.82/13.82 13.82/13.2 13.2/13.2 13.2/13.2 13.2/13.2 13.2/13.2 13.2/13.2 13.2/13.2

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               167.6       240.1       0.652% 0.29%      1.433   1.416 to 1.441    worse (n=10)
cpu_ms                160.0       230.0       0% 0%             1.438   1.438 to 1.438    worse (n=10)
max_rss_kb            4208        2608        2.471% 0%         0.62    0.608 to 0.629    better (n=10)
```

### llvm sort

```
A: llvm.go sort 
B: llvm-sort.xo sort 
10 of 10 rounds used (0 loaded), loads A/B per round: 13.2/13.2 13.2/13.2 13.2/13.2 13.2/12.62 12.62/12.62 12.62/12.62 12.62/12.62 12.62/12.62 12.62/12.62 12.62/12.62

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               275.7       221.4       1.407% 0.242%     0.803   0.788 to 0.806    better (n=10)
cpu_ms                280.0       210.0       0% 0%             0.75    0.75 to 0.75      better (n=10)
max_rss_kb            76000       34624       12.42% 0%         0.456   0.45 to 0.537     better (n=10)
```

### llvm interp

```
A: llvm.go interp 
B: llvm-interp.xo interp 
10 of 10 rounds used (0 loaded), loads A/B per round: 12.62/12.62 12.62/12.62 12.62/12.62 12.62/12.62 12.62/12.62 12.62/12.01 12.01/12.01 12.01/12.01 12.01/12.01 12.01/12.01

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               149.1       72.19       2.782% 6.414%     0.484   0.472 to 0.517    better (n=10)
cpu_ms                140.0       60.0        0% 12.5%          0.429   0.429 to 0.5      better (n=10)
max_rss_kb            4216        2608        3.321% 1.074%     0.619   0.606 to 0.631    better (n=10)
```

### llvm collatz

```
A: llvm.go collatz 
B: llvm-collatz.xo collatz 
10 of 10 rounds used (0 loaded), loads A/B per round: 12.01/12.01 12.01/12.01 12.01/12.01 11.29/11.29 11.29/11.29 11.29/11.29 11.29/11.29 11.29/11.29 10.7/10.7 10.7/10.7

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               518.7       491.0       2.984% 2.601%     0.947   0.928 to 0.976    better (n=10)
cpu_ms                500.0       480.0       2.0% 1.563%       0.96    0.941 to 0.97     better (n=10)
max_rss_kb            4216        2624        2.751% 1.22%      0.622   0.616 to 0.636    better (n=10)
```

### llvm strings

```
A: llvm.go strings 
B: llvm-strings.xo strings 
10 of 10 rounds used (0 loaded), loads A/B per round: 10.7/10.7 10.7/10.7 10.7/10.7 10.7/10.7 10.7/10.7 10.7/10.7 10.7/10.7 10.7/10.7 10.7/10.7 10.7/10.7

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               101.7       44.86       4.879% 3.782%     0.441   0.411 to 0.457    better (n=10)
cpu_ms                130.0       40.0        0% 25.0%          0.308   0.231 to 0.308    better (n=10)
max_rss_kb            25240       15296       4.564% 0%         0.606   0.582 to 0.621    better (n=10)
```

### coll putget

```
A: coll.go putget 
B: coll.xo putget 
10 of 10 rounds used (0 loaded), loads A/B per round: 10.7/10.7 10.7/10.7 10.7/10.7 10.7/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               53.86       32.14       11.57% 5.362%     0.597   0.53 to 0.615     better (n=10)
cpu_ms                40.0        20.0        18.75% 0%         0.5     0.4 to 0.5        too coarse (n=10)
max_rss_kb            8792        7432        2.32% 0.377%      0.845   0.828 to 0.866    better (n=10)
```

### coll get

```
A: coll.go get 
B: coll.xo get 
10 of 10 rounds used (0 loaded), loads A/B per round: 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               25.42       15.47       15.07% 28.44%     0.609   0.512 to 0.722    better (n=10)
cpu_ms                20.0        10.0        0% 0%             0.5     0.5 to 0.5        too coarse (n=10)
max_rss_kb            7088        6896        0.677% 1.218%     0.973   0.966 to 0.984    better (n=10)
```

### coll push

```
A: coll.go push 
B: coll.xo push 
10 of 10 rounds used (0 loaded), loads A/B per round: 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               15.02       9.957       9.512% 5.8%       0.663   0.613 to 0.693    better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            58096       50936       20.09% 0.094%     0.877   0.746 to 0.936    better (n=10)
```

### coll update

```
A: coll.go update 
B: coll.xo update 
10 of 10 rounds used (0 loaded), loads A/B per round: 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               34.08       29.24       5.637% 7.42%      0.858   0.819 to 0.906    better (n=10)
cpu_ms                20.0        20.0        0% 0%             1.0     1.0 to 1.0        too coarse (n=10)
max_rss_kb            8776        7424        1.55% 0%          0.846   0.835 to 0.853    better (n=10)
```

### coll iter

```
A: coll.go iter 
B: coll.xo iter 
10 of 10 rounds used (0 loaded), loads A/B per round: 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               24.73       6.896       5.962% 19.79%     0.279   0.264 to 0.33     better (n=10)
cpu_ms                20.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            7080        6896        0.621% 0.464%     0.974   0.972 to 0.981    better (n=10)
```

### coll copymut

```
A: coll.go copymut 
B: coll.xo copymut 
10 of 10 rounds used (0 loaded), loads A/B per round: 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               16.47       10.2        22.56% 10.59%     0.619   0.52 to 0.691     better (n=10)
cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=10)
max_rss_kb            10112       6912        0.316% 0%         0.684   0.667 to 0.691    better (n=10)
```

### coll branch

```
A: coll.go branch 
B: coll.xo branch 
10 of 10 rounds used (0 loaded), loads A/B per round: 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 10.49/10.49 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               254.2       5.326       15.98% 7.84%      0.021   0.019 to 0.022    better (n=10)
cpu_ms                390.0       0           8.333% 0%         0       0 to 0            better (n=10)
max_rss_kb            18128       6912        48.63% 0.174%     0.381   0.246 to 0.441    better (n=10)
```

### conc spawn

```
A: conc.go spawn 
B: conc-spawn.xo spawn 
10 of 10 rounds used (0 loaded), loads A/B per round: 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               63.1        62.14       1.542% 5.521%     0.985   0.934 to 1.019    within noise (n=10)
cpu_ms                200.0       235.0       5.0% 11.7%        1.175   1.079 to 1.3      worse (n=10)
max_rss_kb            11008       7384        1.817% 2.654%     0.671   0.649 to 0.679    better (n=10)
```

### conc fanout

```
A: conc.go fanout 
B: conc-fanout.xo fanout 
10 of 10 rounds used (0 loaded), loads A/B per round: 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               34.11       30.22       16.24% 11.64%     0.886   0.775 to 0.977    better (n=10)
cpu_ms                80.0        100.0       21.88% 10.0%      1.25    1.056 to 1.429    too coarse (n=10)
max_rss_kb            11584       7344        7.528% 3.704%     0.634   0.586 to 0.655    better (n=10)
```

### conc pingpong

```
A: conc.go pingpong 
B: conc-pingpong.xo pingpong 
10 of 10 rounds used (0 loaded), loads A/B per round: 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97 9.97/9.97

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               25.37       38.32       28.8% 7.249%      1.51    1.273 to 1.811    worse (n=10)
cpu_ms                15.0        30.0        66.67% 0%         2.0     1.2 to 3.0        too coarse (n=10)
max_rss_kb            4248        2720        1.601% 1.176%     0.64    0.632 to 0.65     better (n=10)
```

### conc mutex

```
A: conc.go mutex 
B: conc-mutex.xo mutex 
10 of 10 rounds used (0 loaded), loads A/B per round: 9.97/9.97 9.97/9.97 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               20.52       11.48       13.0% 10.24%      0.559   0.539 to 0.634    better (n=10)
cpu_ms                65.0        10.0        73.08% 100.0%     0.154   0 to 0.5          too coarse (n=10)
max_rss_kb            4792        3328        2.087% 4.447%     0.694   0.686 to 0.723    better (n=10)
```

### conc rwmutex

```
A: conc.go rwmutex 
B: conc-rwmutex.xo rwmutex 
10 of 10 rounds used (0 loaded), loads A/B per round: 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               11.68       44.12       13.97% 2.605%     3.778   3.467 to 4.134    worse (n=10)
cpu_ms                10.0        115.0       0% 8.696%         11.5    10.5 to 12.0      worse (n=10)
max_rss_kb            4912        4448        4.072% 1.799%     0.906   0.891 to 0.938    better (n=10)
```

### conc pmap

```
A: conc.go pmap 
B: conc-pmap.xo pmap 
10 of 10 rounds used (0 loaded), loads A/B per round: 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57 11.57/11.57

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               68.41       71.99       36.24% 20.85%     1.052   0.859 to 1.177    within noise (n=10)
cpu_ms                565.0       605.0       1.77% 2.893%      1.071   1.053 to 1.091    worse (n=10)
max_rss_kb            6728        5216        2.2% 3.681%       0.775   0.753 to 0.793    better (n=10)
```

