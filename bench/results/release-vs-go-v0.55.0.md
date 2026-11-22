# Xo against Go: xo 0.55.0 darwin/arm64 (2026-11-21)

scripts/release-vs-go.sh on Mac Studio (Mac17,15), Apple M5 Ultra, 30 cores, 96 GB, Darwin arm64,
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

## Compile and size

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB | edit's path |
|---|---|---|---|---|---|---|---|---|
| llvm | go | 1400 | 50 | 40 | 110 | 347 / 29 / 69 | 2.39 | - |
| coll | go | 1380 | 40 | 40 | 120 | 355 / 30 / 69 | 2.43 | - |
| conc | go | 1340 | 40 | 40 | 100 | 341 / 30 / 72 | 2.40 | - |
| llvm-fib | xo | 1160 | 60 | 20 | 50 | 344 / 9 / 53 | 1.63 | separate, std records |
| llvm-sieve | xo | 1030 | 50 | 10 | 60 | 343 / 9 / 53 | 1.63 | separate, std records |
| llvm-nbody | xo | 13110 | 70 | 20 | 70 | 331 / 9 / 54 | 1.64 | separate, std records |
| llvm-sort | xo | 1300 | 60 | 30 | 80 | 345 / 10 / 53 | 1.63 | separate, std records |
| llvm-interp | xo | 1540 | 70 | 20 | 70 | 346 / 9 / 54 | 1.64 | separate, std records |
| llvm-collatz | xo | 1390 | 70 | 20 | 70 | 344 / 9 / 53 | 1.63 | separate, std records |
| llvm-strings | xo | 1950 | 70 | 20 | 60 | 345 / 9 / 53 | 1.63 | separate, std records |
| coll | xo | 1190 | 80 | 10 | 70 | 352 / 9 / 64 | 1.82 | separate, std records |
| conc-spawn | xo | 1180 | 50 | 20 | 50 | 357 / 9 / 54 | 1.69 | separate, std records |
| conc-fanout | xo | 1100 | 50 | 10 | 50 | 352 / 9 / 54 | 1.69 | separate, std records |
| conc-pingpong | xo | 1050 | 50 | 10 | 50 | 362 / 9 / 59 | 1.72 | separate, std records |
| conc-mutex | xo | 1060 | 50 | 10 | 50 | 359 / 9 / 54 | 1.75 | separate, std records |
| conc-rwmutex | xo | 1050 | 50 | 10 | 50 | 361 / 9 / 55 | 1.75 | separate, std records |
| conc-pmap | xo | 1050 | 50 | 10 | 50 | 355 / 9 / 55 | 1.69 | separate, std records |

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
| llvm-fib | xo | 930 | 50 | 40 | 50 | 343 / 52 / 53 | 1.63 | separate, std records |
| llvm-sieve | xo | 910 | 80 | 70 | 50 | 343 / 52 / 53 | 1.63 | separate, std records |
| llvm-nbody | xo | 1090 | 60 | 50 | 50 | 345 / 52 / 54 | 1.64 | separate, std records |
| llvm-sort | xo | 970 | 50 | 50 | 50 | 343 / 52 / 54 | 1.63 | separate, std records |
| llvm-interp | xo | 920 | 50 | 50 | 50 | 341 / 52 / 53 | 1.64 | separate, std records |
| llvm-collatz | xo | 880 | 50 | 50 | 50 | 343 / 52 / 53 | 1.63 | separate, std records |
| llvm-strings | xo | 900 | 50 | 50 | 50 | 344 / 52 / 54 | 1.63 | separate, std records |
| coll | xo | 990 | 60 | 60 | 60 | 354 / 62 / 64 | 1.82 | separate, std records |
| conc-spawn | xo | 980 | 50 | 50 | 50 | 357 / 52 / 55 | 1.69 | separate, std records |
| conc-fanout | xo | 980 | 50 | 50 | 50 | 356 / 52 / 54 | 1.69 | separate, std records |
| conc-pingpong | xo | 1110 | 60 | 60 | 60 | 367 / 58 / 59 | 1.72 | separate, std records |
| conc-mutex | xo | 1020 | 50 | 50 | 50 | 357 / 52 / 55 | 1.75 | separate, std records |
| conc-rwmutex | xo | 1280 | 50 | 50 | 50 | 355 / 52 / 55 | 1.75 | separate, std records |
| conc-pmap | xo | 2090 | 50 | 50 | 50 | 363 / 52 / 55 | 1.69 | separate, std records |

### Where xo's warm memory goes (coll, run cache off)

- the bare process (`xo version`): 5.0 MB peak RSS
- the warm build: 62.7 MB peak RSS; with the run cache on (its own directory, a hit) 9.9 MB

The warm build's phases (heap in use and the peak so far, --phase-times), the steps that move them:

```
phase check.setup 3.67 ms heap_mb=20 peak_mb=20
phase check.prefetch 0.35 ms heap_mb=20 peak_mb=20
phase check.dirs 7.44 ms heap_mb=28 peak_mb=29
phase check.parse_list 0.03 ms heap_mb=28 peak_mb=29
phase check 11.60 ms heap_mb=28 peak_mb=29
phase unparse.scans 0.16 ms heap_mb=28 peak_mb=29
phase unparse 0.01 ms heap_mb=28 peak_mb=29
phase fe.keys 1.05 ms heap_mb=28 peak_mb=29
phase fe.read 0.35 ms heap_mb=29 peak_mb=29
phase fe.build 7.52 ms heap_mb=48 peak_mb=53
cache fe 17/23 bodies
cache stdx used
phase ssa 9.06 ms heap_mb=48 peak_mb=53
phase tz 0.00 ms heap_mb=48 peak_mb=53
phase front_end_freed 2.44 ms heap_mb=31 peak_mb=53
phase prune_unwind.unwind 0.12 ms heap_mb=31 peak_mb=53
phase prune_unwind.fold 0.00 ms heap_mb=31 peak_mb=53
phase prune_unwind 0.13 ms heap_mb=31 peak_mb=53
phase cln.keys 1.01 ms heap_mb=31 peak_mb=53
phase cln.read 0.80 ms heap_mb=32 peak_mb=53
phase add_landings 0.01 ms heap_mb=32 peak_mb=53
phase cleanup 0.00 ms heap_mb=32 peak_mb=53
phase cln.store 0.24 ms heap_mb=32 peak_mb=53
phase specialize_args 0.15 ms heap_mb=32 peak_mb=53
phase devirt_once 0.02 ms heap_mb=32 peak_mb=53
phase insert_rc.borrows 0.69 ms heap_mb=32 peak_mb=53
phase insert_rc.uniq_var_params 0.17 ms heap_mb=32 peak_mb=53
phase insert_rc.types 0.15 ms heap_mb=32 peak_mb=53
phase insert_rc.keys 0.20 ms heap_mb=32 peak_mb=53
phase insert_rc.plan 1.22 ms heap_mb=32 peak_mb=53
```

The heap in use at the front end's peak (XO_PROF=heap at fe.build): none in this report. The heap-profiled build of xo 0.55.0 exited 139 (a segfault, task #658), and the script then stopped before its run and test sections; those ran in a second run (SECTIONS="run test", 2026-11-22, same xo and Go), below. The script now reports a failed profiled build and goes on (58546ef1bf).


## A large program: the Azure compute client (armcompute)

A program that links armcompute and makes its VirtualMachines client: Xo's port (xo-lang/azure
resourcemanager/compute/armcompute, built in its workspace) against Go's armcompute v9.0.0
(module cache, offline, -trimpath). Each tool with its default caches in this run's directories: cold
empties them all except, in the first xo row, std's variants; edit appends one function to main.
Wall ms / peak RSS MB, one build each (Mac, 2026-11-22, xo 0.55.0, go1.26.11). Go ran once beside
each xo row; both are listed.

| side | cold | second | warm | edit | second edit | size MB |
|---|---|---|---|---|---|---|
| go (beside the first xo row) | 4766 / 920 | 102 / 44 | 102 / 43 | 331 / 214 | - | 9.30 |
| go (beside the second xo row) | 5578 / 893 | 115 / 42 | 119 / 44 | 352 / 216 | - | 9.30 |
| xo, std's variants already built | 816 / 481 | 55 / 17 | 51 / 17 | 235 / 158 | 262 / 157 | 7.99 |
| xo, the first program after install | 2709 / 859 | 56 / 24 | 55 / 23 | 523 / 476 | 214 / 158 | 7.99 |

The first xo row is a user whose earlier builds made std's variants; the second is the first program
a fresh install builds (XO_SEPCOMP_BG=0: its cold build runs before the variants exist). Both programs
ran ("go client ok; xo client ok"). Against 0.50.1 (release-vs-go-v0.50.1.md, 2026-11-13): the cold
builds peak higher (481 against 262 MB with the variants built, 859 against 545 MB after install),
the binary is larger (7.99 against 6.11 MB), and the unchanged builds take 51 to 56 ms against 38 to 46.

## Run (xo bench --ab, A = Go, B = Xo, 3 rounds)

### llvm fib (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### llvm sieve (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### llvm nbody (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### llvm sort (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### llvm interp (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### llvm collatz (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### llvm strings (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### coll putget (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### coll get (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### coll push (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### coll update (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### coll iter (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### coll copymut (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### coll branch (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### conc spawn (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### conc fanout (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### conc pingpong (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### conc mutex (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### conc rwmutex (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
```

### conc pmap (load 5 at start)

```
xo bench: run --ab as its own HEAVY_CLASS=measure job (a gate or slot would deadlock against a measure window); this run is heavy.sh's heavy.2
(failed)
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
B: ~/xo-lead/notes/xo-4c/t645/mac/xo-0.55.0-darwin-arm64/bin/xo test --std-module=base64 XO_STD=xostd
4 of 4 rounds used (0 loaded), loads A/B per round: 5.72/5.58 5.58/5.58 5.58/6.73 6.73/6.73

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1835        1202        1.188% 0.439%     0.655   0.648 to 0.668    0.654 0.656     better (n=4)
cpu_ms                14058       2323        1.399% 0.761%     0.165   0.163 to 0.168    0.166 0.165     better (n=4)
max_rss_kb            347976      570176      2.694% 0.345%     1.639   1.599 to 1.712    1.66 1.632      worse (n=4)
load: median 5.65 (A) / 6.155 (B); loaded: 0 of 4 rounds (at or above 8.72; baseline 5.72 + 3.0, at least 6.0)
```

### base64 (encoding/base64) edit (load 5 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: goroot/bin/go test -count=1 encoding/base64 
B: ~/xo-lead/notes/xo-4c/t645/mac/xo-0.55.0-darwin-arm64/bin/xo test --std-module=base64 XO_STD=xostd
4 of 4 rounds used (0 loaded), loads A/B per round: 5.98/5.98 6.46/6.46 6.46/6.46 6.46/6.46

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               377.0       190.8       0.979% 5.188%     0.506   0.498 to 0.57     0.499 0.544     better (n=4)
cpu_ms                1358        216.5       1.322% 11.81%     0.159   0.149 to 0.182    0.157 0.168     better (n=4)
max_rss_kb            120480      147120      0.88% 1.468%      1.221   1.204 to 1.278    1.217 1.251     worse (n=4)
load: median 6.46 (A) / 6.46 (B); loaded: 0 of 4 rounds (at or above 8.98; baseline 5.98 + 3.0, at least 6.0)
```

### json (encoding/json) cold (load 5 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: goroot/bin/go test -count=1 encoding/json 
B: ~/xo-lead/notes/xo-4c/t645/mac/xo-0.55.0-darwin-arm64/bin/xo test --std-module=native/json XO_STD=xostd
4 of 4 rounds used (0 loaded), loads A/B per round: 5.12/6.07 6.07/6.07 7.11/7.42 7.42/7.42

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               3355        1341        0.99% 1.986%      0.4     0.397 to 0.415    0.396 0.409     better (n=4)
cpu_ms                29311       2926        1.208% 3.356%     0.1     0.098 to 0.107    0.1 0.102       better (n=4)
max_rss_kb            352456      583472      3.28% 0.571%      1.655   1.54 to 1.71      1.639 1.638     worse (n=4)
load: median 6.59 (A) / 6.745 (B); loaded: 0 of 4 rounds (at or above 8.31; baseline 5.31 + 3.0, at least 6.0)
```

### json (encoding/json) edit (load 5 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
xo bench: --rounds=3 runs 4 rounds: an even count runs each side first equally often
A: goroot/bin/go test -count=1 encoding/json 
B: ~/xo-lead/notes/xo-4c/t645/mac/xo-0.55.0-darwin-arm64/bin/xo test --std-module=native/json XO_STD=xostd
4 of 4 rounds used (0 loaded), loads A/B per round: 5.72/5.72 5.72/5.72 5.72/5.72 5.5/5.72

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               739.2       256.6       1.464% 1.448%     0.347   0.338 to 0.357    0.346 0.352     better (n=4)
cpu_ms                2910        584.8       0.723% 4.345%     0.201   0.185 to 0.207    0.203 0.194     better (n=4)
max_rss_kb            186264      253824      0.511% 0.704%     1.363   1.356 to 1.392    1.363 1.377     worse (n=4)
load: median 5.72 (A) / 5.72 (B); loaded: 0 of 4 rounds (at or above 8.72; baseline 5.72 + 3.0, at least 6.0)
```

