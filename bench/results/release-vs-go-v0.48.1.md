# Xo against Go: xo 0.48.1 darwin/arm64 (2026-11-11)

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
xo's builds run with the whole-build run cache off (XO_BIN_CACHE=off), so the second and
warm rows are compiles through the back-end caches, not a cached result (before 2026-10-29
they could be the run cache's 10 to 20 ms).
Size is the default build's binary.

This report combines two runs of scripts/release-vs-go.sh on the same Mac the same morning, SECTIONS="compile
test" then SECTIONS="test": in the first, both base64 test rows failed on xo's side (`no std module
native/base64`: 0.48.0 merged std/base64 with its implementation, decision 0588, and the script still named
native/base64), so its test section is the second run's, with the script fixed in the same commit. The compile
section is the first run's. The run and http sections were not run (the perf page's "where Xo is behind" rows
are compile and test rows).


## Compile and size

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB | edit's path |
|---|---|---|---|---|---|---|---|---|
| llvm | go | 1300 | 40 | 40 | 90 | 335 / 29 / 70 | 2.39 | - |
| coll | go | 1320 | 40 | 40 | 120 | 347 / 30 / 70 | 2.43 | - |
| conc | go | 1270 | 40 | 40 | 100 | 348 / 29 / 69 | 2.40 | - |
| llvm-fib | xo | 980 | 40 | 30 | 40 | 217 / 44 / 45 | 1.45 | separate, std records |
| llvm-sieve | xo | 990 | 40 | 30 | 30 | 217 / 44 / 45 | 1.45 | separate, std records |
| llvm-nbody | xo | 970 | 40 | 40 | 40 | 225 / 44 / 46 | 1.45 | separate, std records |
| llvm-sort | xo | 970 | 40 | 30 | 40 | 218 / 44 / 46 | 1.45 | separate, std records |
| llvm-interp | xo | 970 | 40 | 30 | 40 | 217 / 44 / 46 | 1.45 | separate, std records |
| llvm-collatz | xo | 950 | 40 | 30 | 30 | 219 / 44 / 45 | 1.45 | separate, std records |
| llvm-strings | xo | 980 | 40 | 30 | 40 | 220 / 44 / 45 | 1.45 | separate, std records |
| coll | xo | 1030 | 60 | 40 | 50 | 224 / 55 / 57 | 1.63 | separate, std records |
| conc-spawn | xo | 1100 | 40 | 30 | 30 | 217 / 44 / 46 | 1.51 | separate, std records |
| conc-fanout | xo | 1110 | 40 | 30 | 30 | 217 / 44 / 46 | 1.54 | separate, std records |
| conc-pingpong | xo | 1140 | 40 | 30 | 40 | 219 / 51 / 52 | 1.55 | separate, std records |
| conc-mutex | xo | 1110 | 40 | 30 | 40 | 216 / 44 / 46 | 1.57 | separate, std records |
| conc-rwmutex | xo | 1130 | 40 | 30 | 40 | 216 / 44 / 46 | 1.59 | separate, std records |
| conc-pmap | xo | 1130 | 40 | 30 | 30 | 217 / 44 / 46 | 1.54 | separate, std records |

The Go column builds one binary per group (llvm, coll, conc); the Xo
programs are one per workload except coll.

## Test (xo bench --ab, A = go test, B = xo test --std-module, 3 rounds)

Cold empties every cache before each round (Go with -count=1); edit appends one
function to the module under test before each round (Go -count=1). Copies of GOROOT
and the release's std.

### base64 (encoding/base64) cold (load 5 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/base64 
B: xo-v0.48.1-darwin-arm64/bin/xo test --std-module=base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.28/5.28 6.46/6.46 6.34/6.46

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1778        1195        0.564% 0.125%     0.672   0.668 to 0.677    0.669 0.674     better (n=3)
cpu_ms                13535       2016        0.63% 0.244%      0.149   0.149 to 0.15     0.149 0.149     better (n=3)
max_rss_kb            337456      306528      2.56% 0.094%      0.908   0.884 to 0.928    0.882 0.92      better (n=3)
load: median 6.34 (A) / 6.46 (B); loaded: 0 of 3 rounds (at or above 8.65; baseline 5.65 + 3.0, at least 6.0)
```

### base64 (encoding/base64) edit (load 5 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/base64 
B: xo-v0.48.1-darwin-arm64/bin/xo test --std-module=base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.11/5.11 5.11/5.11 4.78/4.78

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               293.6       102.9       0.453% 1.686%     0.35    0.347 to 0.358    0.348 0.36      better (n=3)
cpu_ms                1180        104.4       0.611% 0.988%     0.088   0.087 to 0.09     0.089 0.089     better (n=3)
max_rss_kb            120880      117744      0.523% 0.51%      0.974   0.971 to 0.982    0.978 0.974     better (n=3)
load: median 5.11 (A) / 5.11 (B); loaded: 0 of 3 rounds (at or above 8.11; baseline 5.11 + 3.0, at least 6.0)
```

### json (encoding/json) cold (load 4 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/json 
B: xo-v0.48.1-darwin-arm64/bin/xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.64/4.64 4.35/4.35 4.08/4.08

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               3238        1397        0.388% 0.943%     0.431   0.426 to 0.436    0.432 0.428     better (n=3)
cpu_ms                29004       2536        0.193% 1.009%     0.087   0.086 to 0.088    0.087 0.086     better (n=3)
max_rss_kb            355312      326304      1.623% 0.074%     0.918   0.916 to 0.946    0.947 0.917     better (n=3)
load: median 4.35 (A) / 4.35 (B); loaded: 0 of 3 rounds (at or above 7.78; baseline 4.78 + 3.0, at least 6.0)
```

### json (encoding/json) edit (load 5 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/json 
B: xo-v0.48.1-darwin-arm64/bin/xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.69/5.69 5.69/5.69 5.55/5.69

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               706.1       142.0       0.895% 1.976%     0.201   0.197 to 0.206    0.205 0.201     better (n=3)
cpu_ms                2795        200.5       2.071% 6.347%     0.072   0.065 to 0.074    0.071 0.071     better (n=3)
max_rss_kb            186432      134096      0.502% 0.805%     0.719   0.71 to 0.72      0.714 0.718     better (n=3)
load: median 5.69 (A) / 5.69 (B); loaded: 0 of 3 rounds (at or above 8.69; baseline 5.69 + 3.0, at least 6.0)
```
