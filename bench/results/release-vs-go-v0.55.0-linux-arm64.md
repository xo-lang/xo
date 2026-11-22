# Xo against Go: xo 0.55.0 linux/arm64 (2026-11-22)

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
The compile section ran as three host 1 leases of 30 minutes or less (RVG_PART=1/3, 2/3, 3/3,
each row in one part), merged here in the whole section's order.

## Compile and size

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB | edit's path |
|---|---|---|---|---|---|---|---|---|
| llvm | go | 1340 | 19 | 18 | 72 | 313 / 24 / 71 | 2.30 | - |
| coll | go | 1380 | 21 | 20 | 81 | 299 / 23 / 70 | 2.31 | - |
| conc | go | 1366 | 20 | 19 | 70 | 290 / 21 / 68 | 2.30 | - |
| llvm-fib | xo | 1247 | 75 | 16 | 80 | 394 / 13 / 66 | 2.51 | separate, std records |
| llvm-sieve | xo | 1301 | 65 | 8 | 70 | 399 / 13 / 69 | 2.51 | separate, std records |
| llvm-nbody | xo | 1259 | 81 | 11 | 96 | 401 / 13 / 68 | 2.51 | separate, std records |
| llvm-sort | xo | 1316 | 66 | 13 | 72 | 394 / 13 / 69 | 2.51 | separate, std records |
| llvm-interp | xo | 1218 | 70 | 13 | 68 | 402 / 13 / 69 | 2.52 | separate, std records |
| llvm-collatz | xo | 1266 | 62 | 9 | 59 | 401 / 13 / 69 | 2.51 | separate, std records |
| llvm-strings | xo | 1281 | 70 | 13 | 70 | 392 / 13 / 70 | 2.51 | separate, std records |
| coll | xo | 1298 | 115 | 11 | 96 | 404 / 13 / 76 | 2.62 | separate, std records |
| conc-spawn | xo | 1360 | 69 | 15 | 65 | 416 / 13 / 72 | 2.58 | separate, std records |
| conc-fanout | xo | 1396 | 88 | 14 | 77 | 419 / 13 / 73 | 2.59 | separate, std records |
| conc-pingpong | xo | 1338 | 109 | 13 | 86 | 427 / 13 / 74 | 2.61 | separate, std records |
| conc-mutex | xo | 1385 | 81 | 11 | 70 | 430 / 13 / 72 | 2.64 | separate, std records |
| conc-rwmutex | xo | 1368 | 73 | 21 | 89 | 432 / 13 / 73 | 2.64 | separate, std records |
| conc-pmap | xo | 1363 | 73 | 11 | 66 | 412 / 13 / 73 | 2.59 | separate, std records |

Both tools with their default caches (owner, 2026-11-13): cold empties every cache of each
(Go's GOCACHE; xo's back-end caches and its run cache), so second and warm are what a user
sees: Go's up-to-date check, xo's run cache. The Go column builds one binary per group
(llvm, coll, conc); the Xo programs are one per workload except coll.

## xo compile path (run cache off)

A diagnostic, not a comparison with Go: xo with its whole-build run cache off
(XO_BIN_CACHE=off), so second and warm compile through the back-end caches (the
headline table of the reports before 2026-11-13).

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB | edit's path |
|---|---|---|---|---|---|---|---|---|
| llvm-fib | xo | 1262 | 60 | 76 | 73 | 401 / 67 / 68 | 2.51 | separate, std records |
| llvm-sieve | xo | 1238 | 66 | 74 | 69 | 397 / 69 / 68 | 2.51 | separate, std records |
| llvm-nbody | xo | 1208 | 59 | 59 | 77 | 393 / 68 / 70 | 2.51 | separate, std records |
| llvm-sort | xo | 1214 | 71 | 61 | 73 | 398 / 68 / 68 | 2.51 | separate, std records |
| llvm-interp | xo | 1269 | 73 | 58 | 75 | 400 / 70 / 69 | 2.52 | separate, std records |
| llvm-collatz | xo | 1252 | 60 | 58 | 67 | 395 / 68 / 69 | 2.51 | separate, std records |
| llvm-strings | xo | 1227 | 58 | 63 | 63 | 398 / 68 / 67 | 2.51 | separate, std records |
| coll | xo | 1302 | 118 | 78 | 97 | 402 / 76 / 76 | 2.62 | separate, std records |
| conc-spawn | xo | 1330 | 67 | 67 | 85 | 429 / 72 / 72 | 2.58 | separate, std records |
| conc-fanout | xo | 1319 | 72 | 87 | 77 | 412 / 72 / 71 | 2.59 | separate, std records |
| conc-pingpong | xo | 1288 | 84 | 85 | 89 | 420 / 72 / 72 | 2.61 | separate, std records |
| conc-mutex | xo | 1285 | 63 | 66 | 65 | 409 / 77 / 73 | 2.64 | separate, std records |
| conc-rwmutex | xo | 1316 | 73 | 74 | 68 | 419 / 76 / 73 | 2.64 | separate, std records |
| conc-pmap | xo | 1364 | 64 | 60 | 75 | 425 / 73 / 72 | 2.59 | separate, std records |

### Where xo's warm memory goes (coll, run cache off)

- the bare process (`xo version`): 8.2 MB peak RSS
- the warm build: 75.6 MB peak RSS; with the run cache on (its own directory, a hit) 13.5 MB

The warm build's phases (heap in use and the peak so far, --phase-times), the steps that move them:

```
phase check.setup 5.91 ms heap_mb=34 peak_mb=34
phase check.prefetch 3.44 ms heap_mb=34 peak_mb=34
phase check.dirs 9.41 ms heap_mb=42 peak_mb=42
phase check.parse_list 0.01 ms heap_mb=42 peak_mb=42
phase check 18.86 ms heap_mb=42 peak_mb=42
phase unparse.scans 0.19 ms heap_mb=42 peak_mb=42
phase unparse 0.02 ms heap_mb=42 peak_mb=42
phase fe.keys 1.37 ms heap_mb=42 peak_mb=42
phase fe.read 0.42 ms heap_mb=42 peak_mb=42
phase fe.build 12.64 ms heap_mb=57 peak_mb=62
cache fe 17/23 bodies
cache stdx used
phase ssa 14.67 ms heap_mb=57 peak_mb=62
phase tz 0.00 ms heap_mb=57 peak_mb=62
phase front_end_freed 3.58 ms heap_mb=44 peak_mb=62
phase prune_unwind.unwind 0.24 ms heap_mb=44 peak_mb=62
phase prune_unwind.fold 0.00 ms heap_mb=44 peak_mb=62
phase prune_unwind 0.25 ms heap_mb=44 peak_mb=62
phase cln.keys 1.30 ms heap_mb=44 peak_mb=62
phase cln.read 1.20 ms heap_mb=44 peak_mb=62
phase add_landings 0.02 ms heap_mb=44 peak_mb=62
phase cleanup 0.00 ms heap_mb=44 peak_mb=62
phase cln.store 0.58 ms heap_mb=44 peak_mb=62
phase specialize_args 0.38 ms heap_mb=44 peak_mb=62
phase devirt_once 0.02 ms heap_mb=44 peak_mb=62
phase insert_rc.borrows 1.08 ms heap_mb=44 peak_mb=62
phase insert_rc.uniq_var_params 0.25 ms heap_mb=44 peak_mb=62
phase insert_rc.types 0.17 ms heap_mb=44 peak_mb=62
phase insert_rc.keys 0.42 ms heap_mb=44 peak_mb=62
phase insert_rc.plan 1.94 ms heap_mb=44 peak_mb=62
```

The heap in use at the front end's peak (XO_PROF=heap at fe.build, every byte sampled), the top 12:

```
(no heap profile: xo 0.55.0 dies of SIGSEGV in heapos.region_rel_cover in this build, task #658)
```

## Run (xo bench --ab, A = Go, B = Xo, 3 rounds)

The run section ran as two host 1 leases of 30 minutes or less (RVG_PART=1/2, 2/2), merged here in order.

### llvm fib (load 0 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go fib 
B: llvm-fib.xo fib 
4 of 4 rounds used (0 loaded), loads A/B per round: 0.14/0.14 0.14/0.14 0.14/0.14 0.14/0.14

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               239.7       224.4       0.422% 1.857%     0.936   0.923 to 0.97     0.935 0.947     better (n=4)
cpu_ms                240.4       224.3       0.387% 1.941%     0.933   0.919 to 0.968    0.932 0.943     better (n=4)
max_rss_kb            5700        944.0       9.175% 0%         0.166   0.165 to 0.259    0.165 0.202     better (n=4)
load: median 0.14 (A) / 0.14 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.14 + 3.0, at least 6.0)
```

### llvm sieve (load 0 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go sieve 
B: llvm-sieve.xo sieve 
4 of 4 rounds used (0 loaded), loads A/B per round: 0.37/0.37 0.37/0.37 0.37/0.37 0.37/0.37

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               185.7       223.1       15.08% 1.726%     1.201   1.053 to 1.288    1.295 1.076     worse (n=4)
cpu_ms                207.4       223.1       7.017% 1.724%     1.075   0.949 to 1.105    1.109 0.998     within noise (n=4)
max_rss_kb            48098       18376       3.74% 0%          0.382   0.373 to 0.395    0.38 0.385      better (n=4)
load: median 0.37 (A) / 0.37 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.14 + 3.0, at least 6.0)
```

### llvm nbody (load 0 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go nbody 
B: llvm-nbody.xo nbody 
4 of 4 rounds used (0 loaded), loads A/B per round: 0.37/0.37 0.37/0.37 0.37/0.37 0.37/0.37

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               176.9       195.9       5.256% 15.72%     1.107   1.02 to 1.357     1.135 1.111     worse (n=4)
cpu_ms                175.0       195.1       4.12% 12.85%      1.115   1.048 to 1.258    1.138 1.092     worse (n=4)
max_rss_kb            5704        1076        0.14% 2.788%      0.189   0.168 to 0.19     0.189 0.179     better (n=4)
load: median 0.37 (A) / 0.37 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.37 + 3.0, at least 6.0)
```

### llvm sort (load 0 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go sort 
B: llvm-sort.xo sort 
4 of 4 rounds used (0 loaded), loads A/B per round: 1.46/1.46 1.46/1.46 1.46/1.46 1.46/1.46

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               381.3       276.8       4.392% 1.035%     0.726   0.633 to 0.76     0.729 0.691     better (n=4)
cpu_ms                372.9       271.1       5.776% 4.391%     0.727   0.67 to 0.767     0.734 0.723     better (n=4)
max_rss_kb            116664      28110       3.868% 0.374%     0.241   0.225 to 0.259    0.241 0.241     better (n=4)
load: median 1.46 (A) / 1.46 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 0.37 + 3.0, at least 6.0)
```

### llvm interp (load 1 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go interp 
B: llvm-interp.xo interp 
4 of 4 rounds used (0 loaded), loads A/B per round: 1.46/1.46 1.46/1.46 1.46/1.46 1.46/1.46

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               141.9       42.49       4.061% 9.647%     0.3     0.278 to 0.332    0.308 0.293     better (n=4)
cpu_ms                141.5       42.3        3.861% 10.01%     0.299   0.279 to 0.33     0.307 0.292     better (n=4)
max_rss_kb            4694        1012        44.91% 12.65%     0.216   0.165 to 0.295    0.176 0.278     better (n=4)
load: median 1.46 (A) / 1.46 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 1.46 + 3.0, at least 6.0)
```

### llvm collatz (load 1 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go collatz 
B: llvm-collatz.xo collatz 
4 of 4 rounds used (0 loaded), loads A/B per round: 1.59/1.59 1.59/1.59 1.59/1.59 1.59/1.59

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               507.8       464.4       1.07% 0.08%       0.914   0.888 to 0.92     0.918 0.902     better (n=4)
cpu_ms                508.8       464.3       1.086% 0.082%     0.913   0.886 to 0.919    0.916 0.899     better (n=4)
max_rss_kb            3648        944.0       14.12% 0%         0.259   0.166 to 0.259    0.259 0.202     better (n=4)
load: median 1.59 (A) / 1.59 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 1.59 + 3.0, at least 6.0)
```

### llvm strings (load 2 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: llvm.go strings 
B: llvm-strings.xo strings 
4 of 4 rounds used (0 loaded), loads A/B per round: 2.82/2.82 2.82/2.82 2.82/2.82 2.82/2.82

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               208.2       57.12       25.8% 30.38%      0.274   0.264 to 0.283    0.283 0.265     better (n=4)
cpu_ms                248.9       51.15       17.84% 19.7%      0.205   0.192 to 0.214    0.224 0.184     better (n=4)
max_rss_kb            25480       25764       8.493% 0.004%     1.011   0.962 to 1.163    1.097 0.974     within noise (n=4)
load: median 2.82 (A) / 2.82 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 2.82 + 3.0, at least 6.0)
```

### coll putget (load 2 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go putget 
B: coll.xo putget 
4 of 4 rounds used (0 loaded), loads A/B per round: 2.82/2.82 2.82/2.82 2.82/2.82 2.82/2.82

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               78.1        44.7        5.367% 4.26%      0.572   0.553 to 0.719    0.592 0.6       too short (n=4)
cpu_ms                70.04       37.0        13.56% 2.874%     0.528   0.498 to 0.734    0.605 0.526     too coarse (n=4)
max_rss_kb            10004       5610        5.158% 0.143%     0.561   0.467 to 0.562    0.509 0.562     better (n=4)
load: median 2.82 (A) / 2.82 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 2.82 + 3.0, at least 6.0)
```

### coll get (load 2 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go get 
B: coll.xo get 
4 of 4 rounds used (0 loaded), loads A/B per round: 2.82/2.82 2.82/2.82 2.82/2.82 2.82/2.82

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               34.68       19.44       17.7% 18.86%      0.561   0.429 to 0.881    0.723 0.513     too short (n=4)
cpu_ms                32.14       18.31       10.29% 5.247%     0.57    0.485 to 0.657    0.634 0.532     too coarse (n=4)
max_rss_kb            7794        5344        0.077% 0.599%     0.686   0.669 to 0.686    0.677 0.686     better (n=4)
load: median 2.82 (A) / 2.82 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 2.82 + 3.0, at least 6.0)
```

### coll push (load 2 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go push 
B: coll.xo push 
4 of 4 rounds used (0 loaded), loads A/B per round: 2.82/2.82 2.82/2.82 2.82/2.82 2.82/2.82

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               81.57       19.52       6.42% 26.67%      0.239   0.213 to 0.36     0.231 0.308     too short (n=4)
cpu_ms                21.55       13.37       6.798% 20.9%      0.62    0.488 to 1.139    0.62 0.771      too coarse (n=4)
max_rss_kb            51944       43136       2.372% 0.074%     0.83    0.771 to 0.841    0.799 0.836     better (n=4)
load: median 2.82 (A) / 2.82 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 2.82 + 3.0, at least 6.0)
```


### coll update (load 1 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go update 
B: coll.xo update 
4 of 4 rounds used (0 loaded), loads A/B per round: 1.5/1.5 1.5/1.5 1.5/1.5 1.5/1.5

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               30.21       17.82       5.223% 8.401%     0.59    0.533 to 0.716    0.633 0.59      too short (n=4)
cpu_ms                30.86       17.75       4.655% 8.403%     0.575   0.521 to 0.699    0.619 0.575     too coarse (n=4)
max_rss_kb            9980        5628        0.09% 0%          0.564   0.563 to 0.564    0.564 0.564     better (n=4)
load: median 1.5 (A) / 1.5 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 1.5 + 3.0, at least 6.0)
```

### coll iter (load 1 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go iter 
B: coll.xo iter 
4 of 4 rounds used (0 loaded), loads A/B per round: 1.5/1.5 1.5/1.5 1.5/1.5 1.5/1.5

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               20.53       4.759       2.527% 6.039%     0.232   0.216 to 0.242    0.229 0.232     too short (n=4)
cpu_ms                20.76       4.711       2.483% 6.724%     0.227   0.209 to 0.236    0.224 0.226     too coarse (n=4)
max_rss_kb            7786        5216        6.717% 0%         0.67    0.669 to 0.909    0.67 0.771      better (n=4)
load: median 1.5 (A) / 1.5 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 1.5 + 3.0, at least 6.0)
```

### coll copymut (load 1 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go copymut 
B: coll.xo copymut 
4 of 4 rounds used (0 loaded), loads A/B per round: 1.5/1.5 1.5/1.5 1.5/1.5 1.5/1.5

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               12.1        7.483       5.779% 7.318%     0.618   0.585 to 0.67     0.591 0.664     too short (n=4)
cpu_ms                13.4        7.401       2.206% 7.189%     0.552   0.536 to 0.592    0.528 0.597     too coarse (n=4)
max_rss_kb            10206       5216        0.705% 0%         0.511   0.51 to 0.516     0.513 0.511     better (n=4)
load: median 1.5 (A) / 1.5 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 1.5 + 3.0, at least 6.0)
```

### coll branch (load 1 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: coll.go branch 
B: coll.xo branch 
4 of 4 rounds used (0 loaded), loads A/B per round: 1.5/1.5 1.5/1.5 1.5/1.5 1.5/1.5

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               346.1       3.485       1.592% 2.245%     0.01    0.01 to 0.01      0.01 0.01       better (n=4)
cpu_ms                510.0       3.388       3.926% 2.656%     0.007   0.007 to 0.007    0.007 0.007     better (n=4)
max_rss_kb            14616       5216        8.949% 0%         0.357   0.314 to 0.417    0.332 0.387     better (n=4)
load: median 1.5 (A) / 1.5 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 1.5 + 3.0, at least 6.0)
```

### conc spawn (load 1 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: conc.go spawn 
B: conc-spawn.xo spawn 
4 of 4 rounds used (0 loaded), loads A/B per round: 1.5/1.5 1.5/1.5 1.5/1.5 1.5/1.5

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               51.97       17.48       1.269% 10.48%     0.336   0.316 to 0.358    0.332 0.344     too short (n=4)
cpu_ms                136.3       21.07       0.453% 31.94%     0.155   0.123 to 0.186    0.149 0.16      better (n=4)
max_rss_kb            14258       2308        0.316% 40.21%     0.162   0.112 to 0.236    0.148 0.189     better (n=4)
load: median 1.5 (A) / 1.5 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 1.5 + 3.0, at least 6.0)
```

### conc fanout (load 1 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: conc.go fanout 
B: conc-fanout.xo fanout 
4 of 4 rounds used (0 loaded), loads A/B per round: 1.5/1.5 1.5/1.5 1.5/1.5 1.5/1.5

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               29.15       14.44       8.052% 1.747%     0.495   0.46 to 0.555     0.501 0.498     too short (n=4)
cpu_ms                69.67       16.05       9.179% 6.977%     0.23    0.203 to 0.265    0.219 0.243     too coarse (n=4)
max_rss_kb            13208       2628        14.15% 13.39%     0.199   0.15 to 0.229     0.176 0.209     better (n=4)
load: median 1.5 (A) / 1.5 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 1.5 + 3.0, at least 6.0)
```

### conc pingpong (load 1 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: conc.go pingpong 
B: conc-pingpong.xo pingpong 
4 of 4 rounds used (0 loaded), loads A/B per round: 1.46/1.46 1.46/1.46 1.46/1.46 1.46/1.46

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               17.4        15.62       1.529% 3.121%     0.898   0.839 to 0.921    0.892 0.887     too short (n=4)
cpu_ms                19.39       15.75       1.266% 2.93%      0.812   0.761 to 0.834    0.809 0.801     too coarse (n=4)
max_rss_kb            5644        968.0       9.497% 0%         0.172   0.17 to 0.27      0.171 0.21      better (n=4)
load: median 1.46 (A) / 1.46 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 1.46 + 3.0, at least 6.0)
```

### conc mutex (load 1 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: conc.go mutex 
B: conc-mutex.xo mutex 
4 of 4 rounds used (0 loaded), loads A/B per round: 1.46/1.46 1.46/1.46 1.46/1.46 1.46/1.46

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               12.55       3.627       9.639% 4.603%     0.289   0.227 to 0.344    0.306 0.26      too short (n=4)
cpu_ms                36.21       3.586       16.75% 4.239%     0.099   0.071 to 0.119    0.107 0.084     too coarse (n=4)
max_rss_kb            6674        972.0       30.75% 0%         0.146   0.126 to 0.172    0.146 0.146     better (n=4)
load: median 1.46 (A) / 1.46 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 1.46 + 3.0, at least 6.0)
```

### conc rwmutex (load 1 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: conc.go rwmutex 
B: conc-rwmutex.xo rwmutex 
4 of 4 rounds used (0 loaded), loads A/B per round: 1.46/1.46 1.46/1.46 1.46/1.46 1.46/1.46

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               4.969       5.578       2.103% 0.461%     1.123   1.103 to 1.147    1.139 1.105     too short (n=4)
cpu_ms                5.524       5.563       4.014% 0.225%     1.007   0.982 to 1.044    1.021 0.997     too coarse (n=4)
max_rss_kb            5680        972.0       9.93% 0%          0.171   0.126 to 0.172    0.146 0.171     better (n=4)
load: median 1.46 (A) / 1.46 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 1.46 + 3.0, at least 6.0)
```

### conc pmap (load 1 at start)

```
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: conc.go pmap 
B: conc-pmap.xo pmap 
4 of 4 rounds used (0 loaded), loads A/B per round: 1.46/1.46 1.46/1.46 1.46/1.46 1.46/1.46

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               54.34       55.81       0.729% 0.23%      1.027   1.014 to 1.038    1.031 1.023     too short (n=4)
cpu_ms                567.9       575.1       0.247% 0.1%       1.013   1.005 to 1.014    1.014 1.009     worse (n=4)
max_rss_kb            9752        2248        5.301% 0%         0.231   0.23 to 0.292     0.23 0.258      better (n=4)
load: median 1.46 (A) / 1.46 (B); loaded: 0 of 4 rounds (at or above 6.0; baseline 1.46 + 3.0, at least 6.0)
```


## Test (xo bench --ab, A = go test, B = xo test --std-module, 3 rounds)

Cold empties every cache before each round (Go with -count=1); edit appends one
function to the module under test before each round (Go -count=1). Copies of GOROOT
and the release's std.

### base64 (encoding/base64) cold (load 5 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: goroot/bin/go test -count=1 encoding/base64 
B: /work/x4-rvg0550/xo/xo-0.55.0-linux-arm64/bin/xo test --std-module=base64 XO_STD=/work/x4-rvg0550/xo/xo-0.55.0-linux-arm64/bin/../lib/xo/std
4 of 4 rounds used (0 loaded), loads A/B per round: 5.23/5.23 4.89/5.23 4.89/4.89 4.58/4.58

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1811        1475        1.025% 0.337%     0.814   0.809 to 0.831    0.81 0.825      better (n=4)
cpu_ms                9900        3003        0.776% 0.295%     0.303   0.3 to 0.305      0.3 0.305       better (n=4)
max_rss_kb            311926      645516      2.858% 2.413%     2.069   2.022 to 2.286    2.06 2.165      worse (n=4)
load: median 4.89 (A) / 5.06 (B); loaded: 0 of 4 rounds (at or above 8.6; baseline 5.6 + 3.0, at least 6.0)
```

### base64 (encoding/base64) edit (load 5 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: goroot/bin/go test -count=1 encoding/base64 
B: /work/x4-rvg0550/xo/xo-0.55.0-linux-arm64/bin/xo test --std-module=base64 XO_STD=/work/x4-rvg0550/xo/xo-0.55.0-linux-arm64/bin/../lib/xo/std
4 of 4 rounds used (0 loaded), loads A/B per round: 5.88/5.88 5.88/5.88 5.88/5.88 5.88/5.88

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               247.9       12.86       1.689% 4.787%     0.052   0.049 to 0.053    0.052 0.052     better (n=4)
cpu_ms                724.1       40.38       3.558% 1.904%     0.056   0.055 to 0.056    0.055 0.056     better (n=4)
max_rss_kb            116700      116698      0.015% 0.009%     1.0     1.0 to 1.0        1.0 1.0         within noise (n=4)
load: median 5.88 (A) / 5.88 (B); loaded: 0 of 4 rounds (at or above 8.88; baseline 5.88 + 3.0, at least 6.0)
```

### json (encoding/json) cold (load 5 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: goroot/bin/go test -count=1 encoding/json 
B: /work/x4-rvg0550/xo/xo-0.55.0-linux-arm64/bin/xo test --std-module=native/json XO_STD=/work/x4-rvg0550/xo/xo-0.55.0-linux-arm64/bin/../lib/xo/std
4 of 4 rounds used (0 loaded), loads A/B per round: 5.73/5.35 5.0/5.35 4.68/4.68 4.71/4.71

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               3653        1633        0.485% 0.618%     0.447   0.443 to 0.449    0.446 0.448     better (n=4)
cpu_ms                21337       3682        0.473% 0.122%     0.173   0.171 to 0.174    0.173 0.172     better (n=4)
max_rss_kb            304960      655706      2.224% 0.241%     2.15    2.047 to 2.169    2.097 2.161     worse (n=4)
load: median 4.855 (A) / 5.03 (B); loaded: 0 of 4 rounds (at or above 8.88; baseline 5.88 + 3.0, at least 6.0)
```

### json (encoding/json) edit (load 5 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: goroot/bin/go test -count=1 encoding/json 
B: /work/x4-rvg0550/xo/xo-0.55.0-linux-arm64/bin/xo test --std-module=native/json XO_STD=/work/x4-rvg0550/xo/xo-0.55.0-linux-arm64/bin/../lib/xo/std
4 of 4 rounds used (0 loaded), loads A/B per round: 5.07/4.83 4.83/4.83 4.83/4.83 4.83/4.83

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               744.3       13.36       1.199% 5.827%     0.018   0.017 to 0.02     0.018 0.018     better (n=4)
cpu_ms                1935        41.57       1.055% 5.091%     0.021   0.02 to 0.023     0.022 0.021     better (n=4)
max_rss_kb            157932      116686      1.806% 0.01%      0.739   0.723 to 0.744    0.734 0.739     better (n=4)
load: median 4.83 (A) / 4.83 (B); loaded: 0 of 4 rounds (at or above 8.07; baseline 5.07 + 3.0, at least 6.0)
```

