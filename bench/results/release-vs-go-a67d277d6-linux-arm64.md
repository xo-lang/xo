# Xo against Go: xo 0.45.0 linux/arm64 (2026-11-10)

scripts/release-vs-go.sh on Linux host, 12 cores, 39 GB, Linux aarch64,
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


Measured 2026-11-10 on linux/arm64 offload host 1 (Linux aarch64, 12 CPUs, 39 GB; an exclusive lease, so
no other job ran), xo built from main 6fe10a957 with this script's Linux support (4d66f9752), Go 1.26.1.
This is the 0.46.0 cycle's measurement; the 0.46.0 candidate (2517a0b66) is later main (44b668bdd).
Xo and Go alternate in each round (xo bench --ab: A then B, B then A). The size column is empty: the
script read GNU stat's file system text as the size (fixed in the commit that adds this file). The
darwin rows of this release are measured separately on the Mac.

## Compile and size

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB | edit's path |
|---|---|---|---|---|---|---|---|---|
| llvm | go | 1299 | 20 | 19 | 71 | 296 / 21 / 70 |  | - |
| coll | go | 1348 | 20 | 20 | 80 | 292 / 21 / 70 |  | - |
| conc | go | 1304 | 20 | 19 | 69 | 280 / 21 / 72 |  | - |
| llvm-fib | xo | 1126 | 47 | 40 | 40 | 236 / 58 / 56 |  | separate, std records |
| llvm-sieve | xo | 1151 | 47 | 41 | 42 | 229 / 59 / 57 |  | separate, std records |
| llvm-nbody | xo | 1141 | 54 | 45 | 47 | 238 / 59 / 58 |  | separate, std records |
| llvm-sort | xo | 1147 | 51 | 42 | 43 | 233 / 57 / 57 |  | separate, std records |
| llvm-interp | xo | 1150 | 55 | 46 | 48 | 227 / 57 / 60 |  | separate, std records |
| llvm-collatz | xo | 1151 | 50 | 42 | 42 | 224 / 59 / 57 |  | separate, std records |
| llvm-strings | xo | 1148 | 48 | 42 | 42 | 235 / 59 / 57 |  | separate, std records |
| coll | xo | 1205 | 117 | 108 | 111 | 226 / 66 / 65 |  | separate, std records |
| conc-spawn | xo | 1340 | 50 | 43 | 44 | 219 / 61 / 60 |  | separate, std records |
| conc-fanout | xo | 1342 | 50 | 44 | 44 | 219 / 60 / 60 |  | separate, std records |
| conc-pingpong | xo | 1325 | 53 | 48 | 48 | 238 / 59 / 59 |  | separate, std records |
| conc-mutex | xo | 1343 | 52 | 46 | 48 | 219 / 61 / 59 |  | separate, std records |
| conc-rwmutex | xo | 1333 | 52 | 45 | 46 | 226 / 62 / 60 |  | separate, std records |
| conc-pmap | xo | 1341 | 51 | 45 | 46 | 220 / 59 / 61 |  | separate, std records |

The Go column builds one binary per group (llvm, coll, conc); the Xo
programs are one per workload except coll.

## Run (xo bench --ab, A = Go, B = Xo, 3 rounds)

### llvm fib (load 5 at start)

```
A: llvm.go fib 
B: llvm-fib.xo fib 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.68/5.68 5.68/5.31 5.31/5.31

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               232.0       207.0       0.02% 0.292%      0.892   0.887 to 0.893    0.892 0.888     better (n=3)
cpu_ms                232.3       206.9       0.024% 0.292%     0.89    0.886 to 0.891    0.891 0.886     better (n=3)
max_rss_kb            3652        1252        28.09% 0%         0.343   0.219 to 0.343    0.343 0.268     better (n=3)
load: median 5.68 (A) / 5.31 (B); loaded: 0 of 3 rounds (at or above 8.68; baseline 5.68 + 3.0, at least 6.0)
```

### llvm sieve (load 5 at start)

```
A: llvm.go sieve 
B: llvm-sieve.xo sieve 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.31/5.31 5.31/5.31 5.31/5.31

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               162.3       193.3       1.599% 1.766%     1.191   1.152 to 1.231    1.178 1.185     worse (n=3)
cpu_ms                182.8       193.2       4.179% 1.525%     1.057   0.992 to 1.11     1.012 1.062     within noise (n=3)
max_rss_kb            49120       18232       0.615% 0.285%     0.371   0.371 to 0.378    0.372 0.373     better (n=3)
load: median 5.31 (A) / 5.31 (B); loaded: 0 of 3 rounds (at or above 8.31; baseline 5.31 + 3.0, at least 6.0)
```

### llvm nbody (load 5 at start)

```
A: llvm.go nbody 
B: llvm-nbody.xo nbody 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.31/5.31 5.31/5.31 5.31/5.31

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               159.1       162.3       0.022% 0.221%     1.02    1.018 to 1.023    1.021 1.02      worse (n=3)
cpu_ms                159.5       162.3       0.05% 0.223%      1.017   1.016 to 1.02     1.019 1.017     worse (n=3)
max_rss_kb            3648        1252        28.34% 0%         0.343   0.219 to 0.343    0.219 0.343     better (n=3)
load: median 5.31 (A) / 5.31 (B); loaded: 0 of 3 rounds (at or above 8.31; baseline 5.31 + 3.0, at least 6.0)
```

### llvm sort (load 5 at start)

```
A: llvm.go sort 
B: llvm-sort.xo sort 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.31/5.31 5.31/5.31 4.96/4.96

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               269.2       208.5       0.112% 0.374%     0.775   0.77 to 0.778     0.774 0.776     better (n=3)
cpu_ms                284.1       208.5       0.53% 0.378%      0.734   0.723 to 0.735    0.732 0.733     better (n=3)
max_rss_kb            133436      27912       6.019% 0%         0.209   0.192 to 0.216    0.209 0.204     better (n=3)
load: median 5.31 (A) / 5.31 (B); loaded: 0 of 3 rounds (at or above 8.31; baseline 5.31 + 3.0, at least 6.0)
```

### llvm interp (load 4 at start)

```
A: llvm.go interp 
B: llvm-interp.xo interp 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.96/4.96 4.96/4.96 4.96/4.96

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               125.5       39.68       0.634% 6.652%     0.316   0.282 to 0.323    0.3 0.317       better (n=3)
cpu_ms                125.7       39.61       0.67% 6.656%      0.315   0.281 to 0.321    0.298 0.316     better (n=3)
max_rss_kb            3648        1252        28.13% 0%         0.343   0.22 to 0.343     0.343 0.268     better (n=3)
load: median 4.96 (A) / 4.96 (B); loaded: 0 of 3 rounds (at or above 7.96; baseline 4.96 + 3.0, at least 6.0)
```

### llvm collatz (load 4 at start)

```
A: llvm.go collatz 
B: llvm-collatz.xo collatz 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.96/4.96 4.96/4.96 4.96/4.96

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               445.1       397.8       0.057% 0.177%     0.894   0.893 to 0.897    0.894 0.894     better (n=3)
cpu_ms                445.6       397.8       0.049% 0.173%     0.893   0.892 to 0.896    0.893 0.893     better (n=3)
max_rss_kb            5768        1280        0.589% 1.25%      0.222   0.219 to 0.222    0.219 0.223     better (n=3)
load: median 4.96 (A) / 4.96 (B); loaded: 0 of 3 rounds (at or above 7.96; baseline 4.96 + 3.0, at least 6.0)
```

### llvm strings (load 4 at start)

```
A: llvm.go strings 
B: llvm-strings.xo strings 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.96/4.96 4.96/4.64 4.64/4.64

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               152.7       43.91       2.581% 0.325%     0.288   0.281 to 0.298    0.297 0.285     better (n=3)
cpu_ms                248.2       43.85       4.401% 0.302%     0.177   0.173 to 0.191    0.19 0.176      better (n=3)
max_rss_kb            26256       15384       5.477% 0%         0.586   0.559 to 0.624    0.624 0.572     better (n=3)
load: median 4.96 (A) / 4.64 (B); loaded: 0 of 3 rounds (at or above 7.96; baseline 4.96 + 3.0, at least 6.0)
```

### coll putget (load 4 at start)

```
A: coll.go putget 
B: coll.xo putget 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.64/4.64 4.64/4.64 4.64/4.64

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               46.93       28.87       5.461% 5.539%     0.615   0.615 to 0.621    0.583 0.658     better (n=3)
cpu_ms                47.42       28.84       5.702% 5.564%     0.608   0.608 to 0.615    0.575 0.651     too coarse (n=3)
max_rss_kb            7924        5360        13.02% 0%         0.676   0.537 to 0.677    0.537 0.677     better (n=3)
load: median 4.64 (A) / 4.64 (B); loaded: 0 of 3 rounds (at or above 7.64; baseline 4.64 + 3.0, at least 6.0)
```

### coll get (load 4 at start)

```
A: coll.go get 
B: coll.xo get 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.64/4.64 4.64/4.64 4.64/4.64

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               21.89       11.12       3.872% 1.041%     0.508   0.492 to 0.543    0.539 0.505     better (n=3)
cpu_ms                22.05       11.05       4.073% 0.982%     0.501   0.486 to 0.537    0.534 0.498     too coarse (n=3)
max_rss_kb            7788        4944        0.077% 0%         0.635   0.634 to 0.635    0.635 0.634     better (n=3)
load: median 4.64 (A) / 4.64 (B); loaded: 0 of 3 rounds (at or above 7.64; baseline 4.64 + 3.0, at least 6.0)
```

### coll push (load 4 at start)

```
A: coll.go push 
B: coll.xo push 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.64/4.64 4.64/4.64 4.64/4.64

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               15.17       10.67       9.541% 2.515%     0.704   0.693 to 0.864    0.7 0.805       better (n=3)
cpu_ms                24.05       10.62       12.36% 2.407%     0.441   0.429 to 0.573    0.439 0.512     too coarse (n=3)
max_rss_kb            101900      42864       8.881% 0%         0.421   0.407 to 0.491    0.421 0.445     better (n=3)
load: median 4.64 (A) / 4.64 (B); loaded: 0 of 3 rounds (at or above 7.64; baseline 4.64 + 3.0, at least 6.0)
```

### coll update (load 4 at start)

```
A: coll.go update 
B: coll.xo update 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.64/4.64 4.64/4.64 4.64/4.64

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               27.94       16.55       6.716% 0.771%     0.592   0.574 to 0.672    0.589 0.622     better (n=3)
cpu_ms                28.68       16.48       6.814% 0.713%     0.575   0.56 to 0.657     0.573 0.607     too coarse (n=3)
max_rss_kb            9976        5360        0.04% 0%          0.537   0.537 to 0.538    0.538 0.537     better (n=3)
load: median 4.64 (A) / 4.64 (B); loaded: 0 of 3 rounds (at or above 7.64; baseline 4.64 + 3.0, at least 6.0)
```

### coll iter (load 4 at start)

```
A: coll.go iter 
B: coll.xo iter 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.64/4.64 4.64/4.64 4.64/4.64

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               20.24       3.659       1.251% 1.123%     0.181   0.18 to 0.188     0.182 0.183     better (n=3)
cpu_ms                20.48       3.602       1.079% 1.277%     0.176   0.175 to 0.184    0.178 0.177     too coarse (n=3)
max_rss_kb            7792        4944        0.051% 0%         0.634   0.634 to 0.635    0.635 0.634     better (n=3)
load: median 4.64 (A) / 4.64 (B); loaded: 0 of 3 rounds (at or above 7.64; baseline 4.64 + 3.0, at least 6.0)
```

### coll copymut (load 4 at start)

```
A: coll.go copymut 
B: coll.xo copymut 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.64/4.64 4.64/4.64 4.64/4.64

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               10.73       7.412       1.868% 1.157%     0.691   0.671 to 0.703    0.691 0.687     better (n=3)
cpu_ms                12.02       7.358       1.348% 1.4%       0.612   0.597 to 0.621    0.608 0.614     too coarse (n=3)
max_rss_kb            10232       4944        0.782% 0%         0.483   0.481 to 0.489    0.489 0.482     better (n=3)
load: median 4.64 (A) / 4.64 (B); loaded: 0 of 3 rounds (at or above 7.64; baseline 4.64 + 3.0, at least 6.0)
```

### coll branch (load 4 at start)

```
A: coll.go branch 
B: coll.xo branch 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.64/4.64 4.64/4.64 4.64/4.64

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               349.3       3.041       0.187% 1.892%     0.009   0.009 to 0.009    0.009 0.009     better (n=3)
cpu_ms                542.3       2.987       2.525% 0.937%     0.006   0.005 to 0.006    0.005 0.006     better (n=3)
max_rss_kb            14528       4944        7.654% 0%         0.34    0.336 to 0.396    0.336 0.366     better (n=3)
load: median 4.64 (A) / 4.64 (B); loaded: 0 of 3 rounds (at or above 7.64; baseline 4.64 + 3.0, at least 6.0)
```

### conc spawn (load 4 at start)

```
A: conc.go spawn 
B: conc-spawn.xo spawn 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.64/4.64 4.64/4.64 4.64/4.64

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               51.38       19.96       2.771% 0.63%      0.388   0.381 to 0.403    0.388 0.396     better (n=3)
cpu_ms                138.7       25.4        1.975% 5.874%     0.183   0.163 to 0.192    0.174 0.186     better (n=3)
max_rss_kb            14264       2868        0.365% 15.62%     0.201   0.174 to 0.239    0.219 0.175     better (n=3)
load: median 4.64 (A) / 4.64 (B); loaded: 0 of 3 rounds (at or above 7.64; baseline 4.64 + 3.0, at least 6.0)
```

### conc fanout (load 4 at start)

```
A: conc.go fanout 
B: conc-fanout.xo fanout 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.64/4.64 4.64/4.64 4.64/4.64

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               29.13       14.13       2.588% 3.091%     0.485   0.468 to 0.507    0.505 0.483     better (n=3)
cpu_ms                70.81       18.32       3.467% 13.63%     0.259   0.201 to 0.274    0.251 0.257     too coarse (n=3)
max_rss_kb            14232       2740        7.617% 25.69%     0.193   0.142 to 0.219    0.156 0.237     better (n=3)
load: median 4.64 (A) / 4.64 (B); loaded: 0 of 3 rounds (at or above 7.64; baseline 4.64 + 3.0, at least 6.0)
```

### conc pingpong (load 4 at start)

```
A: conc.go pingpong 
B: conc-pingpong.xo pingpong 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.64/4.64 4.64/4.64 4.64/4.64

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               16.74       15.13       1.043% 0.175%     0.904   0.893 to 0.908    0.904 0.9       better (n=3)
cpu_ms                18.67       15.21       0.806% 0.256%     0.815   0.812 to 0.821    0.814 0.817     too coarse (n=3)
max_rss_kb            5644        1248        18.75% 0%         0.221   0.219 to 0.348    0.219 0.27      better (n=3)
load: median 4.64 (A) / 4.64 (B); loaded: 0 of 3 rounds (at or above 7.64; baseline 4.64 + 3.0, at least 6.0)
```

### conc mutex (load 4 at start)

```
A: conc.go mutex 
B: conc-mutex.xo mutex 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.64/4.64 4.64/4.64 4.64/4.64

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               12.52       3.984       12.14% 2.37%      0.318   0.259 to 0.335    0.262 0.321     better (n=3)
cpu_ms                37.34       3.985       19.22% 2.146%     0.107   0.078 to 0.113    0.079 0.108     too coarse (n=3)
max_rss_kb            5656        1248        18.28% 0%         0.221   0.162 to 0.221    0.221 0.187     better (n=3)
load: median 4.64 (A) / 4.64 (B); loaded: 0 of 3 rounds (at or above 7.64; baseline 4.64 + 3.0, at least 6.0)
```

### conc rwmutex (load 4 at start)

```
A: conc.go rwmutex 
B: conc-rwmutex.xo rwmutex 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.64/4.64 4.64/4.64 4.64/4.64

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               5.133       5.783       0.755% 1.278%     1.127   1.122 to 1.134    1.125 1.129     worse (n=3)
cpu_ms                5.62        5.779       2.037% 1.28%      1.028   0.978 to 1.043    1.032 1.009     too coarse (n=3)
max_rss_kb            5732        1236        18.0% 0%          0.216   0.16 to 0.219     0.216 0.185     better (n=3)
load: median 4.64 (A) / 4.64 (B); loaded: 0 of 3 rounds (at or above 7.64; baseline 4.64 + 3.0, at least 6.0)
```

### conc pmap (load 4 at start)

```
A: conc.go pmap 
B: conc-pmap.xo pmap 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.64/4.64 4.64/4.64 4.64/4.64

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               54.55       57.85       1.823% 1.607%     1.06    1.032 to 1.06     1.047 1.047     worse (n=3)
cpu_ms                576.7       577.2       0.603% 0.723%     1.001   1.001 to 1.005    1.007 0.998     worse (n=3)
max_rss_kb            9764        2356        10.65% 2.716%     0.241   0.199 to 0.255    0.248 0.218     better (n=3)
load: median 4.64 (A) / 4.64 (B); loaded: 0 of 3 rounds (at or above 7.64; baseline 4.64 + 3.0, at least 6.0)
```


## Test (xo bench --ab, A = go test, B = xo test --std-module, 3 rounds)

Cold empties every cache before each round (Go with -count=1); edit appends one
function to the module under test before each round (Go -count=1). Copies of GOROOT
and the release's std.

### base64 (encoding/base64) cold (load 4 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/base64 
B: /work/gatel2-rvglinux-4d66f9752/bin/xo test --std-module=native/base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.35/4.35 4.08/4.08 3.84/4.08

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1738        1378        0.281% 1.097%     0.793   0.791 to 0.804    0.802 0.787     better (n=3)
cpu_ms                9548        2332        0.519% 1.242%     0.244   0.241 to 0.248    0.246 0.241     better (n=3)
max_rss_kb            315064      305972      2.672% 0.259%     0.971   0.933 to 0.989    0.986 0.955     better (n=3)
load: median 4.08 (A) / 4.08 (B); loaded: 0 of 3 rounds (at or above 7.35; baseline 4.35 + 3.0, at least 6.0)
```

### base64 (encoding/base64) edit (load 3 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/base64 
B: /work/gatel2-rvglinux-4d66f9752/bin/xo test --std-module=native/base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 3.77/3.77 3.77/3.77 3.55/3.77

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               248.1       221.7       1.706% 0.699%     0.893   0.868 to 0.898    0.867 0.895     better (n=3)
cpu_ms                717.5       268.0       1.537% 9.494%     0.373   0.366 to 0.434    0.39 0.375      better (n=3)
max_rss_kb            108556      210228      0.138% 1.315%     1.937   1.9 to 1.952      1.922 1.937     worse (n=3)
load: median 3.77 (A) / 3.77 (B); loaded: 0 of 3 rounds (at or above 6.77; baseline 3.77 + 3.0, at least 6.0)
```

### json (encoding/json) cold (load 3 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/json 
B: /work/gatel2-rvglinux-4d66f9752/bin/xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 3.42/3.42 3.95/3.95 3.87/3.87

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               3508        1620        0.209% 1.64%      0.462   0.461 to 0.477    0.469 0.461     better (n=3)
cpu_ms                20599       2862        0.063% 1.064%     0.139   0.139 to 0.142    0.14 0.139      better (n=3)
max_rss_kb            309312      331604      1.115% 0.58%      1.072   1.067 to 1.088    1.076 1.071     worse (n=3)
load: median 3.87 (A) / 3.87 (B); loaded: 0 of 3 rounds (at or above 6.55; baseline 3.55 + 3.0, at least 6.0)
```

### json (encoding/json) edit (load 4 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/json 
B: /work/gatel2-rvglinux-4d66f9752/bin/xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.1/4.1 4.1/3.85 3.85/3.85

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               740.0       178.1       2.333% 0.087%     0.241   0.236 to 0.247    0.247 0.238     better (n=3)
cpu_ms                1889        238.4       1.588% 2.008%     0.126   0.123 to 0.127    0.127 0.125     better (n=3)
max_rss_kb            159552      230144      1.271% 1.329%     1.442   1.393 to 1.459    1.427 1.421     worse (n=3)
load: median 4.1 (A) / 3.85 (B); loaded: 0 of 3 rounds (at or above 7.1; baseline 4.1 + 3.0, at least 6.0)
```

