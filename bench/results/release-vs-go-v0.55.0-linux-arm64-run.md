# Xo against Go: xo 0.55.0 linux/arm64 (2026-11-22)

The run section only (task #686), rerun with the REPEAT counts of
scripts/release-vs-go.sh at 98335486d8 (bench/perf's programs as of that
commit) and the published 0.55.0 linux/arm64 archive, on host 1 in two
exclusive leases (SECTIONS=run; WORKLOADS fib to push, then update to
pmap), so every row is long enough for a wall verdict. It replaces the run
section of release-vs-go-v0.55.0-linux-arm64.md, where 12 of 20 rows were
too short. The build and size paragraphs below are the script's standard
header; this file has no compile rows.

scripts/release-vs-go.sh on Linux host, 12 cores, 39 GB, Linux aarch64,
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

### llvm fib, repeat 5 (load 0 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go fib 5 
B: llvm-fib.xo fib 5 
4 of 4 rounds used (0 loaded), loads A/B per round: 0.08/0.08 0.15/0.08 0.15/0.15 0.15/0.15

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1100        973.4       0.1% 0.338%       0.885   0.883 to 0.892    0.889 0.884     better (n=4)
cpu_ms                1101        973.3       0.096% 0.335%     0.884   0.882 to 0.891    0.888 0.884     better (n=4)
max_rss_kb            4670        964.0       44.33% 0%         0.206   0.167 to 0.265    0.168 0.265     better (n=4)
load: median 0.15 (A) / 0.115 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.08 + 3.0, at least 6.0)
```

### llvm sieve, repeat 6 (load 0 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go sieve 6 
B: llvm-sieve.xo sieve 6 
4 of 4 rounds used (0 loaded), loads A/B per round: 0.22/0.22 0.28/0.22 0.28/0.28 0.34/0.28

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               852.9       1151        7.524% 0.506%     1.35    1.236 to 1.402    1.276 1.397     worse (n=4)
cpu_ms                981.6       1151        13.71% 0.502%     1.172   1.033 to 1.257    1.071 1.247     worse (n=4)
max_rss_kb            48134       18400       3.177% 0%         0.382   0.375 to 0.39     0.376 0.389     better (n=4)
load: median 0.28 (A) / 0.25 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.22 + 3.0, at least 6.0)
```

### llvm nbody, repeat 6 (load 0 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go nbody 6 
B: llvm-nbody.xo nbody 6 
4 of 4 rounds used (0 loaded), loads A/B per round: 0.34/0.34 0.39/0.34 0.39/0.39 0.39/0.39

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               914.8       921.2       0.128% 0.175%     1.007   1.003 to 1.008    1.007 1.006     worse (n=4)
cpu_ms                915.5       921.1       0.131% 0.167%     1.006   1.002 to 1.007    1.006 1.005     worse (n=4)
max_rss_kb            4700        968.0       44.96% 0%         0.206   0.168 to 0.266    0.206 0.206     better (n=4)
load: median 0.39 (A) / 0.365 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.34 + 3.0, at least 6.0)
```

### llvm sort, repeat 4 (load 0 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go sort 4 
B: llvm-sort.xo sort 4 
4 of 4 rounds used (0 loaded), loads A/B per round: 0.44/0.44 0.44/0.44 0.49/0.49 0.49/0.49

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1030        781.4       1.184% 0.102%     0.758   0.729 to 0.763    0.76 0.743      better (n=4)
cpu_ms                1110        781.2       2.086% 0.094%     0.704   0.659 to 0.708    0.705 0.681     better (n=4)
max_rss_kb            155850      28084       4.546% 0%         0.18    0.165 to 0.183    0.18 0.173      better (n=4)
load: median 0.465 (A) / 0.465 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.44 + 3.0, at least 6.0)
```

### llvm interp, repeat 24 (load 0 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go interp 24 
B: llvm-interp.xo interp 24 
4 of 4 rounds used (0 loaded), loads A/B per round: 0.53/0.57 0.57/0.57 0.6/0.6 0.63/0.6

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               2843        862.1       1.714% 6.224%     0.303   0.284 to 0.328    0.3 0.303       better (n=4)
cpu_ms                2845        861.9       1.746% 6.22%      0.303   0.283 to 0.328    0.3 0.303       better (n=4)
max_rss_kb            4704        968.0       45.11% 0%         0.206   0.168 to 0.266    0.266 0.168     better (n=4)
load: median 0.585 (A) / 0.585 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.49 + 3.0, at least 6.0)
```

### llvm collatz, repeat 3 (load 0 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go collatz 3 
B: llvm-collatz.xo collatz 3 
4 of 4 rounds used (0 loaded), loads A/B per round: 0.66/0.66 0.69/0.66 0.69/0.69 0.72/0.69

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1328        1321        0.108% 0.192%     0.995   0.992 to 0.996    0.995 0.994     better (n=4)
cpu_ms                1329        1321        0.111% 0.208%     0.994   0.992 to 0.995    0.994 0.993     better (n=4)
max_rss_kb            4674        968.0       44.29% 0%         0.207   0.169 to 0.266    0.266 0.169     better (n=4)
load: median 0.69 (A) / 0.675 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.63 + 3.0, at least 6.0)
```

### llvm strings, repeat 18 (load 0 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go strings 18 
B: llvm-strings.xo strings 18 
4 of 4 rounds used (0 loaded), loads A/B per round: 0.82/0.82 0.82/0.82 0.91/0.91 0.92/0.91

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               2512        655.2       4.192% 1.047%     0.261   0.245 to 0.271    0.26 0.26       better (n=4)
cpu_ms                4049        655.2       4.658% 1.055%     0.162   0.151 to 0.17     0.161 0.162     better (n=4)
max_rss_kb            25936       25788       6.732% 0.093%     0.994   0.834 to 1.012    1.007 0.904     within noise (n=4)
load: median 0.865 (A) / 0.865 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.72 + 3.0, at least 6.0)
```

### coll putget, repeat 23 (load 0 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go putget 23 
B: coll.xo putget 23 
4 of 4 rounds used (0 loaded), loads A/B per round: 0.92/0.93 0.93/0.93 0.93/0.93 0.93/0.93

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1038        498.8       2.545% 2.625%     0.48    0.468 to 0.523    0.496 0.474     better (n=4)
cpu_ms                1045        498.7       2.562% 2.624%     0.477   0.465 to 0.519    0.492 0.471     better (n=4)
max_rss_kb            10464       8060        1.003% 0%         0.77    0.763 to 0.775    0.774 0.765     better (n=4)
load: median 0.93 (A) / 0.93 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.92 + 3.0, at least 6.0)
```

### coll get, repeat 52 (load 0 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go get 52 
B: coll.xo get 52 
4 of 4 rounds used (0 loaded), loads A/B per round: 0.93/0.93 0.93/0.93 0.93/0.94 0.94/0.94

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               859.8       419.6       0.546% 4.973%     0.488   0.441 to 0.505    0.463 0.496     better (n=4)
cpu_ms                860.9       419.5       0.463% 4.962%     0.487   0.441 to 0.504    0.462 0.495     better (n=4)
max_rss_kb            7798        5220        6.63% 0%          0.669   0.669 to 0.909    0.771 0.669     better (n=4)
load: median 0.93 (A) / 0.935 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.93 + 3.0, at least 6.0)
```

### coll push, repeat 52 (load 0 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go push 52 
B: coll.xo push 52 
4 of 4 rounds used (0 loaded), loads A/B per round: 0.94/0.94 0.94/0.94 1.02/1.02 1.02/1.02

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               634.9       379.5       3.762% 1.177%     0.598   0.575 to 0.61     0.614 0.583     better (n=4)
cpu_ms                1076        379.4       4.116% 1.201%     0.353   0.335 to 0.363    0.365 0.342     better (n=4)
max_rss_kb            112104      30584       0.633% 0.121%     0.273   0.271 to 0.276    0.275 0.272     better (n=4)
load: median 0.98 (A) / 0.98 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.94 + 3.0, at least 6.0)
```


### coll update, repeat 57 (load 0 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go update 57 
B: coll.xo update 57 
4 of 4 rounds used (0 loaded), loads A/B per round: 0.88/0.88 0.89/0.89 0.89/0.89 0.9/0.89

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1431        714.9       2.57% 0.863%      0.5     0.492 to 0.548    0.516 0.501     better (n=4)
cpu_ms                1443        714.8       2.522% 0.862%     0.495   0.488 to 0.543    0.511 0.497     better (n=4)
max_rss_kb            11542       8060        15.81% 0%         0.698   0.643 to 0.764    0.646 0.76      better (n=4)
load: median 0.89 (A) / 0.89 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.88 + 3.0, at least 6.0)
```

### coll iter, repeat 211 (load 0 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go iter 211 
B: coll.xo iter 211 
4 of 4 rounds used (0 loaded), loads A/B per round: 0.9/0.9 0.9/0.9 0.91/0.92 0.92/0.92

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               3340        255.0       0.396% 7.323%     0.076   0.068 to 0.089    0.072 0.083     better (n=4)
cpu_ms                3343        254.9       0.397% 7.292%     0.076   0.068 to 0.089    0.072 0.083     better (n=4)
max_rss_kb            7790        5220        0.064% 0%         0.67    0.67 to 0.67      0.67 0.67       better (n=4)
load: median 0.905 (A) / 0.91 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.9 + 3.0, at least 6.0)
```

### coll copymut, repeat 134 (load 0 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go copymut 134 
B: coll.xo copymut 134 
4 of 4 rounds used (0 loaded), loads A/B per round: 0.93/0.93 0.93/0.93 0.93/0.93 0.93/0.93

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1185        507.3       4.895% 3.884%     0.428   0.391 to 0.446    0.417 0.421     better (n=4)
cpu_ms                1274        507.0       5.28% 3.821%      0.398   0.359 to 0.415    0.387 0.39      better (n=4)
max_rss_kb            12234       5220        5.215% 0%         0.427   0.422 to 0.515    0.464 0.427     better (n=4)
load: median 0.93 (A) / 0.93 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.93 + 3.0, at least 6.0)
```

### coll branch, repeat 90 (load 0 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go branch 90 
B: coll.xo branch 90 
4 of 4 rounds used (0 loaded), loads A/B per round: 1.56/2.5 2.5/2.5 2.35/2.33 2.33/2.33

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               28022       33.19       2.658% 2.418%     0.001   0.001 to 0.001    0.001 0.001     better (n=4)
cpu_ms                42096       33.13       2.031% 2.334%     0.001   0.001 to 0.001    0.001 0.001     better (n=4)
max_rss_kb            62930       5220        203.7% 0%         0.083   0.019 to 0.274    0.035 0.084     better (n=4)
load: median 2.34 (A) / 2.415 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.93 + 3.0, at least 6.0)
```

### conc spawn, repeat 58 (load 2 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: conc.go spawn 58 
B: conc-spawn.xo spawn 58 
4 of 4 rounds used (0 loaded), loads A/B per round: 2.33/2.46 2.46/2.46 2.59/2.62 2.62/2.62

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               3071        914.2       0.249% 1.04%      0.298   0.294 to 0.301    0.299 0.297     better (n=4)
cpu_ms                8214        926.1       0.28% 1.215%      0.113   0.111 to 0.114    0.112 0.113     better (n=4)
max_rss_kb            14308       2908        4.019% 5.502%     0.203   0.185 to 0.218    0.197 0.203     better (n=4)
load: median 2.525 (A) / 2.54 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 2.33 + 3.0, at least 6.0)
```

### conc fanout, repeat 70 (load 2 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: conc.go fanout 70 
B: conc-fanout.xo fanout 70 
4 of 4 rounds used (0 loaded), loads A/B per round: 2.81/2.83 2.83/2.83 2.83/2.92 2.92/2.92

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1918        896.2       0.689% 3.883%     0.467   0.467 to 0.535    0.473 0.5       better (n=4)
cpu_ms                4596        900.8       1.648% 3.846%     0.196   0.194 to 0.224    0.199 0.21      better (n=4)
max_rss_kb            13762       3228        16.77% 19.83%     0.235   0.131 to 0.363    0.193 0.277     better (n=4)
load: median 2.83 (A) / 2.875 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 2.81 + 3.0, at least 6.0)
```

### conc pingpong, repeat 65 (load 3 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: conc.go pingpong 65 
B: conc-pingpong.xo pingpong 65 
4 of 4 rounds used (0 loaded), loads A/B per round: 3.25/3.25 3.07/3.25 3.07/3.07 3.07/3.07

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1000        929.7       1.018% 0.583%     0.929   0.929 to 0.942    0.922 0.943     better (n=4)
cpu_ms                1089        931.4       0.973% 0.627%     0.855   0.855 to 0.865    0.848 0.867     better (n=4)
max_rss_kb            5682        932.0       1.144% 0%         0.164   0.163 to 0.165    0.165 0.163     better (n=4)
load: median 3.07 (A) / 3.16 (B); loaded: 0 of 4 rounds (at or above 6.25; baseline 3.25 + 3.0, at least 6.0)
```

### conc mutex, repeat 278 (load 2 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: conc.go mutex 278 
B: conc-mutex.xo mutex 278 
4 of 4 rounds used (0 loaded), loads A/B per round: 2.9/2.99 2.99/2.99 2.99/3.07 2.91/3.07

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               3581        1143        0.6% 1.112%       0.319   0.319 to 0.327    0.324 0.32      better (n=4)
cpu_ms                10894       2015        1.02% 1.541%      0.185   0.185 to 0.19     0.189 0.185     better (n=4)
max_rss_kb            7740        1316        0.943% 2.432%     0.17    0.154 to 0.171    0.161 0.17      better (n=4)
load: median 2.95 (A) / 3.03 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 2.9 + 3.0, at least 6.0)
```

### conc rwmutex, repeat 202 (load 2 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: conc.go rwmutex 202 
B: conc-rwmutex.xo rwmutex 202 
4 of 4 rounds used (0 loaded), loads A/B per round: 3.0/3.0 3.22/3.08 3.22/3.22 3.25/3.2

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               846.3       6697        2.103% 0.226%     7.913   7.784 to 8.0      7.928 7.868     worse (n=4)
cpu_ms                921.1       23781       1.979% 0.204%     25.82   25.01 to 26.11    25.8 25.58      worse (n=4)
max_rss_kb            7718        1960        6.893% 6.531%     0.254   0.244 to 0.336    0.293 0.253     better (n=4)
load: median 3.22 (A) / 3.14 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 2.91 + 3.0, at least 6.0)
```

### conc pmap, repeat 19 (load 3 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: conc.go pmap 19 
B: conc-pmap.xo pmap 19 
4 of 4 rounds used (0 loaded), loads A/B per round: 3.25/3.25 4.03/4.03 4.03/4.03 4.51/4.51

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1037        1061        0.868% 0.842%     1.023   1.016 to 1.033    1.037 1.014     worse (n=4)
cpu_ms                10973       11113       0.367% 0.514%     1.013   1.012 to 1.027    1.022 1.011     worse (n=4)
max_rss_kb            10786       2396        19.4% 5.342%      0.222   0.196 to 0.252    0.207 0.239     better (n=4)
load: median 4.03 (A) / 4.03 (B); loaded: 0 of 4 rounds (at or above 6.25; baseline 3.25 + 3.0, at least 6.0)
```

