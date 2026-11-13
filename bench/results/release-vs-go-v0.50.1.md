# Xo against Go: xo 0.50.1 darwin/arm64 (2026-11-13)

The published v0.50.1 archive (xo-v0.50.1-darwin-arm64.tar.gz, sha256 cad0cbec...; tag 6d0a9dc7c), run by the script as of the commit that adds this
report. Since this report the headline table runs both tools with their default caches (owner, 2026-11-13);
the run-cache-off rows are a diagnostic table below it.

scripts/release-vs-go.sh on Mac Studio (Mac17,15), Apple M5 Ultra, 30 cores, 96 GB, Darwin arm64,
Go go1.26.1. Build times are wall milliseconds of one build; peak MB is the
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

## Compile and size

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB | edit's path |
|---|---|---|---|---|---|---|---|---|
| llvm | go | 1510 | 50 | 40 | 110 | 341 / 29 / 69 | 2.39 | - |
| coll | go | 1400 | 40 | 40 | 120 | 357 / 30 / 71 | 2.43 | - |
| conc | go | 1370 | 40 | 40 | 100 | 345 / 29 / 70 | 2.40 | - |
| llvm-fib | xo | 1160 | 50 | 10 | 50 | 210 / 9 / 87 | 1.45 | separate, std records |
| llvm-sieve | xo | 1110 | 50 | 10 | 50 | 209 / 9 / 87 | 1.45 | separate, std records |
| llvm-nbody | xo | 1150 | 50 | 10 | 50 | 208 / 9 / 88 | 1.45 | separate, std records |
| llvm-sort | xo | 1170 | 50 | 10 | 50 | 214 / 9 / 88 | 1.45 | separate, std records |
| llvm-interp | xo | 1120 | 50 | 10 | 50 | 209 / 9 / 88 | 1.47 | separate, std records |
| llvm-collatz | xo | 1120 | 50 | 10 | 50 | 211 / 9 / 88 | 1.45 | separate, std records |
| llvm-strings | xo | 1150 | 50 | 10 | 50 | 215 / 9 / 88 | 1.45 | separate, std records |
| coll | xo | 1210 | 70 | 10 | 70 | 226 / 9 / 93 | 1.63 | separate, std records |
| conc-spawn | xo | 1370 | 60 | 10 | 50 | 208 / 9 / 91 | 1.53 | separate, std records |
| conc-fanout | xo | 1330 | 50 | 10 | 50 | 210 / 9 / 92 | 1.55 | separate, std records |
| conc-pingpong | xo | 1480 | 50 | 10 | 50 | 210 / 9 / 92 | 1.56 | separate, std records |
| conc-mutex | xo | 1390 | 50 | 10 | 50 | 212 / 9 / 93 | 1.58 | separate, std records |
| conc-rwmutex | xo | 1290 | 50 | 10 | 50 | 213 / 9 / 92 | 1.60 | separate, std records |
| conc-pmap | xo | 1320 | 50 | 10 | 50 | 213 / 9 / 92 | 1.55 | separate, std records |

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
| llvm-fib | xo | 1200 | 40 | 40 | 40 | 215 / 45 / 46 | 1.45 | separate, std records |
| llvm-sieve | xo | 1000 | 40 | 40 | 40 | 210 / 45 / 46 | 1.45 | separate, std records |
| llvm-nbody | xo | 1180 | 40 | 40 | 50 | 216 / 45 / 47 | 1.45 | separate, std records |
| llvm-sort | xo | 1130 | 40 | 40 | 40 | 215 / 45 / 47 | 1.45 | separate, std records |
| llvm-interp | xo | 1090 | 40 | 40 | 40 | 212 / 45 / 47 | 1.47 | separate, std records |
| llvm-collatz | xo | 1070 | 40 | 40 | 40 | 215 / 45 / 46 | 1.45 | separate, std records |
| llvm-strings | xo | 1010 | 40 | 40 | 40 | 219 / 45 / 47 | 1.45 | separate, std records |
| coll | xo | 1140 | 50 | 50 | 50 | 221 / 57 / 58 | 1.63 | separate, std records |
| conc-spawn | xo | 1280 | 40 | 40 | 40 | 214 / 45 / 47 | 1.53 | separate, std records |
| conc-fanout | xo | 1210 | 50 | 50 | 50 | 208 / 45 / 47 | 1.55 | separate, std records |
| conc-pingpong | xo | 1330 | 40 | 40 | 40 | 211 / 53 / 53 | 1.56 | separate, std records |
| conc-mutex | xo | 1230 | 40 | 40 | 40 | 214 / 45 / 47 | 1.58 | separate, std records |
| conc-rwmutex | xo | 1230 | 40 | 40 | 40 | 211 / 45 / 47 | 1.60 | separate, std records |
| conc-pmap | xo | 1140 | 40 | 40 | 40 | 210 / 45 / 48 | 1.55 | separate, std records |

### Where xo's warm memory goes (coll, run cache off)

- the bare process (`xo version`): 4.9 MB peak RSS
- the warm build: 55.8 MB peak RSS; with the run cache on (its own directory, a hit) 9.3 MB

The warm build's phases (heap in use and the peak so far, --phase-times), the steps that move them:

```
phase check.setup 3.25 ms (heap 19 MB, peak 19 MB)
phase check.dirs 10.20 ms (heap 23 MB, peak 23 MB)
phase fe.keys 0.60 ms (heap 27 MB, peak 27 MB)
phase fe.build 6.58 ms (heap 40 MB, peak 45 MB)
cache fe 17/23 bodies
cache stdx used
phase front_end_freed 2.16 ms (heap 24 MB, peak 45 MB)
phase insert_rc.read 0.43 ms (heap 25 MB, peak 45 MB)
phase emit 1.94 ms (heap 22 MB, peak 45 MB)
cache functions 43/242 reused
cache parts 1/1 reused
phase link.key 0.00 ms (heap 21 MB, peak 45 MB)
phase link.inputs 0.12 ms (heap 22 MB, peak 45 MB)
phase link.store_prepare 1.51 ms (heap 26 MB, peak 45 MB)
phase link.layout 0.69 ms (heap 27 MB, peak 45 MB)
phase link.write 1.70 ms (heap 24 MB, peak 45 MB)
phase link.store_bin 0.00 ms (heap 22 MB, peak 45 MB)
phase link 7.27 ms (heap 22 MB, peak 45 MB)
```

The heap in use at the front end's peak (XO_PROF=heap at fe.build, every byte sampled), the top 12:

```
xo heap profile: 84451 samples at one per 1B allocated (estimated totals)
in use: 31.32MB in 75089 objects; allocated: 65.81MB in 195615 objects
      flat  flat%   sum%        cum   cum%  line (the largest)  function
    7.35MB 23.47% 23.47%     7.35MB 23.47%  load.xo:626         check.Checker.own_objs
    6.98MB 22.29% 45.76%     9.38MB 29.94%  export.xo:792       check.decode_std_export
    4.75MB 15.16% 60.92%     4.75MB 15.16%  raw.xo:32           raw.bytes_buf
    1.65MB  5.25% 66.17%     1.98MB  6.32%  -                   xo.map.put[Map[Str, StdFact]]
    1.57MB  5.02% 71.20%     2.25MB  7.20%  export.xo:505       check.ExR.obj
  895.45kB  2.79% 73.99%   895.45kB  2.79%  -                   xo.map.rebuildk[Str]
  799.83kB  2.49% 76.48%   799.83kB  2.49%  export.xo:265       check.ExR.ints
  644.55kB  2.01% 78.49%   856.71kB  2.67%  parser.xo:1727      parser.Parser.parse_type
  515.61kB  1.61% 80.10%   515.61kB  1.61%  stdfacts.xo:184     ssa.SfR.bools
  475.92kB  1.48% 81.58%   740.95kB  2.31%  rccache.xo:991      ssa.decode_func_at
  395.59kB  1.23% 82.82%   395.59kB  1.23%  export.xo:1241      check.Checker.relocate_export
  280.22kB  0.87% 83.69%   280.22kB  0.87%  -                   xo.map.putk[Int]
```

## A large program: the Azure compute client (armcompute)

A program that links armcompute and makes its VirtualMachines client: Xo's port (xo-lang/azure
resourcemanager/compute/armcompute, 63 files, built in its workspace) against Go's armcompute v9.0.0
(module cache, offline, -trimpath). Each tool with its default caches in this run's directories: cold
empties them all; xo's std variants are built after the cold row; edit appends one function to main.
Wall ms / peak RSS MB, one build each (Mac, 2026-11-13).

| side | cold | second | warm | edit | size MB |
|---|---|---|---|---|---|
| go | 4473 / 829 | 106 / 45 | 107 / 43 | 305 / 213 | 9.28 |
| xo | 3317 / 545 | 44 / 20 | 41 / 20 | 476 / 288 | 6.11 |

xo's edit compiles through the back-end caches (the run cache misses on any changed file), so its edit is
slower than Go's and peaks higher; second and warm are run-cache hits.

Why the edit is slower: the cold build ran before std's variants existed, so it compiled the whole
program, and none of its entries serve separate compilation; the second and warm builds were run-cache
hits, so the edit was the program's first separate build (fe 0/57 bodies, chk 0/9 modules, functions
0/450 reused). Measured after (Mac, 2026-11-13): with the variants built before the cold build, cold
takes 727 ms, the first edit 234 ms (fe 49/57, chk 8/9, functions 175/450) and the next 213 ms, against
Go's 305 ms; a second edit after this table's sequence took 182 to 197 ms. A first run of this row wrote its
logs inside the workspace's directory, which the run cache keys (a directory listing that changes can add
an xo.mod or xo.work), so every build missed: keep logs outside the program's tree when timing.

## Run (xo bench --ab, A = Go, B = Xo, 3 rounds)

### llvm fib (load 5 at start)

```
A: llvm.go fib 
B: llvm-fib.xo fib 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.28/5.28 5.28/5.28 5.28/5.28

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               224.3       245.7       1.006% 2.047%     1.095   1.043 to 1.109    1.084 1.061     worse (n=3)
cpu_ms                223.1       244.5       1.139% 2.075%     1.096   1.038 to 1.108    1.08 1.06       worse (n=3)
max_rss_kb            4720        2976        0.847% 0%         0.631   0.62 to 0.631     0.62 0.631      better (n=3)
load: median 5.28 (A) / 5.28 (B); loaded: 0 of 3 rounds (at or above 8.4; baseline 5.4 + 3.0, at least 6.0)
```

### llvm sieve (load 5 at start)

```
A: llvm.go sieve 
B: llvm-sieve.xo sieve 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.28/5.28 5.28/5.28 5.28/5.28

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               111.7       108.8       1.391% 2.786%     0.974   0.92 to 0.989     0.962 0.968     better (n=3)
cpu_ms                126.3       107.6       5.723% 2.767%     0.852   0.77 to 0.859     0.835 0.809     better (n=3)
max_rss_kb            48784       13584       2.509% 0%         0.278   0.268 to 0.281    0.268 0.28      better (n=3)
load: median 5.28 (A) / 5.28 (B); loaded: 0 of 3 rounds (at or above 8.28; baseline 5.28 + 3.0, at least 6.0)
```

### llvm nbody (load 5 at start)

```
A: llvm.go nbody 
B: llvm-nbody.xo nbody 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.28/5.28 5.28/5.28 5.28/5.28

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               151.0       146.8       0.516% 0.174%     0.972   0.961 to 0.974    0.971 0.97      better (n=3)
cpu_ms                150.3       145.6       0.065% 0.201%     0.969   0.968 to 0.972    0.969 0.973     better (n=3)
max_rss_kb            4800        2976        1.0% 0%           0.62    0.618 to 0.631    0.631 0.619     better (n=3)
load: median 5.28 (A) / 5.28 (B); loaded: 0 of 3 rounds (at or above 8.28; baseline 5.28 + 3.0, at least 6.0)
```

### llvm sort (load 5 at start)

```
A: llvm.go sort 
B: llvm-sort.xo sort 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.28/5.28 5.28/5.26 5.26/5.26

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               264.6       205.9       0.769% 0.234%     0.778   0.772 to 0.782    0.771 0.781     better (n=3)
cpu_ms                278.2       204.7       0.938% 0.292%     0.736   0.729 to 0.741    0.729 0.739     better (n=3)
max_rss_kb            110000      27088       6.298% 0%         0.246   0.225 to 0.255    0.225 0.25      better (n=3)
load: median 5.28 (A) / 5.26 (B); loaded: 0 of 3 rounds (at or above 8.28; baseline 5.28 + 3.0, at least 6.0)
```

### llvm interp (load 5 at start)

```
A: llvm.go interp 
B: llvm-interp.xo interp 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.26/5.26 5.26/5.26 5.26/5.26

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               123.2       40.25       0.68% 7.718%      0.327   0.295 to 0.348    0.318 0.328     better (n=3)
cpu_ms                122.6       38.96       0.672% 7.983%     0.318   0.289 to 0.342    0.313 0.319     better (n=3)
max_rss_kb            4880        2976        0.328% 0%         0.61    0.61 to 0.614     0.61 0.612      better (n=3)
load: median 5.26 (A) / 5.26 (B); loaded: 0 of 3 rounds (at or above 8.26; baseline 5.26 + 3.0, at least 6.0)
```

### llvm collatz (load 5 at start)

```
A: llvm.go collatz 
B: llvm-collatz.xo collatz 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.26/5.26 5.26/5.26 5.26/5.26

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               469.5       428.6       0.123% 0.677%     0.913   0.913 to 0.923    0.911 0.925     better (n=3)
cpu_ms                468.5       427.2       0.14% 0.714%      0.912   0.912 to 0.922    0.91 0.925      better (n=3)
max_rss_kb            4848        2976        1.65% 0.538%      0.614   0.604 to 0.624    0.607 0.619     better (n=3)
load: median 5.26 (A) / 5.26 (B); loaded: 0 of 3 rounds (at or above 8.26; baseline 5.26 + 3.0, at least 6.0)
```

### llvm strings (load 5 at start)

```
A: llvm.go strings 
B: llvm-strings.xo strings 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.08/5.08 5.08/5.08 5.08/5.08

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               87.54       37.76       1.442% 1.86%      0.431   0.425 to 0.441    0.425 0.446     better (n=3)
cpu_ms                139.5       36.41       2.621% 1.692%     0.261   0.259 to 0.272    0.257 0.273     better (n=3)
max_rss_kb            26736       16944       3.92% 0%          0.634   0.601 to 0.649    0.634 0.624     better (n=3)
load: median 5.08 (A) / 5.08 (B); loaded: 0 of 3 rounds (at or above 8.08; baseline 5.08 + 3.0, at least 6.0)
```

### coll putget (load 5 at start)

```
A: coll.go putget 
B: coll.xo putget 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.08/5.08 5.08/5.08 5.08/5.08

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               51.18       26.51       0.873% 0.558%     0.518   0.514 to 0.526    0.519 0.516     better (n=3)
cpu_ms                50.8        25.37       0.792% 0.723%     0.499   0.492 to 0.507    0.499 0.496     too coarse (n=3)
max_rss_kb            9712        6336        0.577% 0%         0.652   0.651 to 0.659    0.651 0.656     better (n=3)
load: median 5.08 (A) / 5.08 (B); loaded: 0 of 3 rounds (at or above 8.08; baseline 5.08 + 3.0, at least 6.0)
```

### coll get (load 5 at start)

```
A: coll.go get 
B: coll.xo get 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.08/5.08 5.08/5.08 5.08/5.08

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               18.83       11.8        0.414% 3.394%     0.627   0.613 to 0.652    0.629 0.628     better (n=3)
cpu_ms                17.94       10.77       0.429% 4.382%     0.601   0.569 to 0.626    0.6 0.598       too coarse (n=3)
max_rss_kb            7680        6288        1.042% 0%         0.819   0.81 to 0.827     0.819 0.819     better (n=3)
load: median 5.08 (A) / 5.08 (B); loaded: 0 of 3 rounds (at or above 8.08; baseline 5.08 + 3.0, at least 6.0)
```

### coll push (load 5 at start)

```
A: coll.go push 
B: coll.xo push 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.08/5.08 5.08/5.08 5.08/5.08

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               16.22       9.298       4.141% 2.226%     0.573   0.551 to 0.59     0.535 0.589     better (n=3)
cpu_ms                19.67       8.013       3.938% 3.251%     0.407   0.407 to 0.413    0.388 0.43      too coarse (n=3)
max_rss_kb            89248       27280       6.257% 0.059%     0.306   0.283 to 0.32     0.283 0.313     better (n=3)
load: median 5.08 (A) / 5.08 (B); loaded: 0 of 3 rounds (at or above 8.08; baseline 5.08 + 3.0, at least 6.0)
```

### coll update (load 5 at start)

```
A: coll.go update 
B: coll.xo update 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.08/5.08 5.08/5.08 5.08/5.08

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               31.12       16.51       0.657% 2.305%     0.53    0.515 to 0.545    0.532 0.521     better (n=3)
cpu_ms                30.76       15.31       0.231% 1.486%     0.498   0.493 to 0.51     0.502 0.494     too coarse (n=3)
max_rss_kb            9920        6336        3.226% 0%         0.639   0.635 to 0.678    0.678 0.637     better (n=3)
load: median 5.08 (A) / 5.08 (B); loaded: 0 of 3 rounds (at or above 8.08; baseline 5.08 + 3.0, at least 6.0)
```

### coll iter (load 5 at start)

```
A: coll.go iter 
B: coll.xo iter 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.08/5.08 5.08/5.08 5.08/5.08

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               19.89       6.368       0.721% 3.871%     0.32    0.298 to 0.324    0.31 0.325      better (n=3)
cpu_ms                18.74       5.162       1.5% 3.884%       0.276   0.255 to 0.28     0.267 0.281     too coarse (n=3)
max_rss_kb            7728        6288        0.518% 0.254%     0.814   0.812 to 0.825    0.814 0.817     better (n=3)
load: median 5.08 (A) / 5.08 (B); loaded: 0 of 3 rounds (at or above 8.08; baseline 5.08 + 3.0, at least 6.0)
```

### coll copymut (load 5 at start)

```
A: coll.go copymut 
B: coll.xo copymut 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.08/5.08 5.08/5.08 5.08/5.08

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               14.86       8.169       1.642% 2.767%     0.55    0.544 to 0.556    0.533 0.57      better (n=3)
cpu_ms                16.91       7.108       3.92% 2.075%      0.42    0.398 to 0.421    0.386 0.429     too coarse (n=3)
max_rss_kb            11984       6288        2.203% 0%         0.525   0.521 to 0.545    0.521 0.535     better (n=3)
load: median 5.08 (A) / 5.08 (B); loaded: 0 of 3 rounds (at or above 8.08; baseline 5.08 + 3.0, at least 6.0)
```

### coll branch (load 5 at start)

```
A: coll.go branch 
B: coll.xo branch 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.08/5.08 5.08/5.08 5.08/5.08

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               275.1       5.55        4.259% 7.243%     0.02    0.018 to 0.022    0.02 0.021      better (n=3)
cpu_ms                643.2       4.234       5.364% 8.137%     0.007   0.006 to 0.007    0.006 0.007     better (n=3)
max_rss_kb            19776       6288        2.832% 0%         0.318   0.314 to 0.332    0.314 0.325     better (n=3)
load: median 5.08 (A) / 5.08 (B); loaded: 0 of 3 rounds (at or above 8.08; baseline 5.08 + 3.0, at least 6.0)
```

### conc spawn (load 5 at start)

```
A: conc.go spawn 
B: conc-spawn.xo spawn 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.08/5.08 5.08/5.08 5.08/5.08

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               102.7       21.49       1.248% 4.85%      0.209   0.195 to 0.218    0.203 0.217     better (n=3)
cpu_ms                367.0       26.43       1.19% 18.95%      0.072   0.063 to 0.092    0.068 0.091     better (n=3)
max_rss_kb            12256       4688        2.415% 2.218%     0.383   0.383 to 0.392    0.377 0.4       better (n=3)
load: median 5.08 (A) / 5.08 (B); loaded: 0 of 3 rounds (at or above 8.08; baseline 5.08 + 3.0, at least 6.0)
```

### conc fanout (load 5 at start)

```
A: conc.go fanout 
B: conc-fanout.xo fanout 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.08/5.08 5.08/5.08 5.08/5.08

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               47.1        13.9        16.36% 2.74%      0.295   0.274 to 0.38     0.397 0.274     better (n=3)
cpu_ms                144.0       12.92       15.16% 2.996%     0.09    0.084 to 0.115    0.12 0.085      better (n=3)
max_rss_kb            12576       5008        5.089% 1.597%     0.398   0.374 to 0.4      0.412 0.374     better (n=3)
load: median 5.08 (A) / 5.08 (B); loaded: 0 of 3 rounds (at or above 8.08; baseline 5.08 + 3.0, at least 6.0)
```

### conc pingpong (load 5 at start)

```
A: conc.go pingpong 
B: conc-pingpong.xo pingpong 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.08/5.08 5.08/5.08 5.08/5.08

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               20.95       16.14       2.356% 2.051%     0.77    0.77 to 0.798     0.783 0.783     better (n=3)
cpu_ms                28.11       15.21       2.381% 0.585%     0.541   0.536 to 0.559    0.543 0.543     too coarse (n=3)
max_rss_kb            4752        3072        2.525% 0%         0.646   0.617 to 0.649    0.617 0.648     better (n=3)
load: median 5.08 (A) / 5.08 (B); loaded: 0 of 3 rounds (at or above 8.08; baseline 5.08 + 3.0, at least 6.0)
```

### conc mutex (load 5 at start)

```
A: conc.go mutex 
B: conc-mutex.xo mutex 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.08/5.08 5.08/5.08 5.08/5.08

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               25.48       6.392       6.647% 1.416%     0.251   0.239 to 0.276    0.278 0.246     better (n=3)
cpu_ms                100.2       5.457       9.228% 3.253%     0.054   0.049 to 0.063    0.06 0.055      better (n=3)
max_rss_kb            5360        3024        0.299% 0%         0.564   0.563 to 0.566    0.563 0.565     better (n=3)
load: median 5.08 (A) / 5.08 (B); loaded: 0 of 3 rounds (at or above 8.08; baseline 5.08 + 3.0, at least 6.0)
```

### conc rwmutex (load 5 at start)

```
A: conc.go rwmutex 
B: conc-rwmutex.xo rwmutex 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.08/5.08 5.08/5.08 5.08/5.08

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               10.25       8.25        2.238% 1.309%     0.805   0.791 to 0.806    0.772 0.825     better (n=3)
cpu_ms                15.78       7.259       2.06% 1.336%      0.46    0.436 to 0.464    0.446 0.453     too coarse (n=3)
max_rss_kb            5472        3040        1.462% 0%         0.556   0.554 to 0.571    0.571 0.555     better (n=3)
load: median 5.08 (A) / 5.08 (B); loaded: 0 of 3 rounds (at or above 8.08; baseline 5.08 + 3.0, at least 6.0)
```

### conc pmap (load 5 at start)

```
A: conc.go pmap 
B: conc-pmap.xo pmap 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.08/5.08 5.08/5.08 5.08/5.08

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               28.26       29.0        0.658% 0.157%     1.026   1.022 to 1.033    1.027 1.025     worse (n=3)
cpu_ms                479.3       489.2       0.037% 0.177%     1.021   1.018 to 1.022    1.021 1.018     worse (n=3)
max_rss_kb            10016       9440        0.559% 0.678%     0.942   0.927 to 0.95     0.936 0.938     better (n=3)
load: median 5.08 (A) / 5.08 (B); loaded: 0 of 3 rounds (at or above 8.08; baseline 5.08 + 3.0, at least 6.0)
```


## Test (xo bench --ab, A = go test, B = xo test --std-module, 3 rounds)

Cold empties every cache before each round (Go with -count=1); edit appends one
function to the module under test before each round (Go -count=1). Copies of GOROOT
and the release's std.

### base64 (encoding/base64) cold (load 4 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/base64 
B: ~/xo-lead/notes/xo-lang-2/rvg0501/darwin/xo-v0.50.1-darwin-arm64/bin/xo test --std-module=base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 7.24/7.24 9.54/9.54 9.26/9.54

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1882        1247        0.537% 1.015%     0.663   0.661 to 0.681    0.668 0.665     better (n=3)
cpu_ms                14029       2112        0.483% 1.593%     0.151   0.147 to 0.153    0.15 0.148      better (n=3)
max_rss_kb            351312      306656      1.507% 0.081%     0.873   0.85 to 0.877     0.873 0.862     better (n=3)
load: median 9.26 (A) / 9.54 (B); loaded: 2 of 3 rounds (at or above 7.99; baseline 4.99 + 3.0, at least 6.0)
warning: the machine was busy through the run (median load at or above 7.99): wall and cpu ratios are not reliable; measure with HEAVY_CLASS=measure (agent-tools.md Measuring) or --max-load
```

### base64 (encoding/base64) edit (load 5 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/base64 
B: ~/xo-lead/notes/xo-lang-2/rvg0501/darwin/xo-v0.50.1-darwin-arm64/bin/xo test --std-module=base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.75/5.75 6.25/6.25 6.25/6.25

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               340.5       127.8       3.635% 5.247%     0.375   0.363 to 0.376    0.355 0.391     better (n=3)
cpu_ms                1311        134.9       1.806% 6.517%     0.103   0.094 to 0.104    0.098 0.104     better (n=3)
max_rss_kb            120688      125968      0.464% 0.648%     1.044   1.038 to 1.047    1.051 1.031     worse (n=3)
load: median 6.25 (A) / 6.25 (B); loaded: 0 of 3 rounds (at or above 8.75; baseline 5.75 + 3.0, at least 6.0)
```

### json (encoding/json) cold (load 5 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/json 
B: ~/xo-lead/notes/xo-lang-2/rvg0501/darwin/xo-v0.50.1-darwin-arm64/bin/xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.22/5.22 6.48/6.28 6.28/6.28

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               3720        1586        4.818% 5.415%     0.426   0.421 to 0.467    0.424 0.484     better (n=3)
cpu_ms                30382       2991        1.585% 2.074%     0.098   0.098 to 0.099    0.097 0.1       better (n=3)
max_rss_kb            352496      335264      1.044% 0.091%     0.951   0.95 to 0.97      0.951 0.961     better (n=3)
load: median 6.28 (A) / 6.28 (B); loaded: 0 of 3 rounds (at or above 8.32; baseline 5.32 + 3.0, at least 6.0)
```

### json (encoding/json) edit (load 5 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/json 
B: ~/xo-lead/notes/xo-lang-2/rvg0501/darwin/xo-v0.50.1-darwin-arm64/bin/xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.87/5.87 5.87/5.87 5.87/5.87

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               747.6       173.4       1.907% 3.211%     0.232   0.223 to 0.232    0.222 0.236     better (n=3)
cpu_ms                2950        268.6       1.246% 2.211%     0.091   0.09 to 0.092     0.09 0.092      better (n=3)
max_rss_kb            186816      142784      0.912% 0.958%     0.764   0.759 to 0.783    0.762 0.769     better (n=3)
load: median 5.87 (A) / 5.87 (B); loaded: 0 of 3 rounds (at or above 8.87; baseline 5.87 + 3.0, at least 6.0)
```

