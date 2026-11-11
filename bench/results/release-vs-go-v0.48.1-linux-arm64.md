# Xo against Go: xo 0.48.1 linux/arm64, compile and test (2026-11-11)

The released 0.48.1 (its linux-arm64 archive, as built and signed for the release) on linux/arm64
offload host 1, SECTIONS="compile test" of scripts/release-vs-go.sh at main f5ec0746b (the test
pairs as MODULE:XO_FILE:GO_PKG:GO_FILE). It replaces, for 0.48.1, the compile rows of
release-vs-go-v0.46.0-linux-arm64-compile.md and the test rows of release-vs-go-6fe10a957-linux-arm64.md
(the run and HTTP sections were not rerun). The run held the host's three slots (exclusive), but
the host's load average was 5 to 9 through it; the bench marks one json cold round of three as
loaded, and its spreads stay under 1%. 0.48.2 differs from 0.48.1 only in the Windows linker, so
these figures hold for it.

Against the earlier host 1 files: coll's warm build 107 to 54 ms and its edit build 112 to 57 ms
(Go 19 and 79); test memory on an edit: base64 1.94 to 0.93 of go test's, json 1.44 to 0.77; on a
cold test run: base64 0.97 to 1.05, json 1.07 to 1.11 (335 against 302 MB).

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

## Compile and size

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB | edit's path |
|---|---|---|---|---|---|---|---|---|
| llvm | go | 1362 | 20 | 19 | 74 | 300 / 21 / 70 | 2.29 | - |
| coll | go | 1356 | 21 | 18 | 79 | 288 / 21 / 70 | 2.30 | - |
| conc | go | 1343 | 20 | 19 | 68 | 299 / 23 / 68 | 2.30 | - |
| llvm-fib | xo | 1165 | 48 | 42 | 40 | 231 / 56 / 54 | 2.14 | separate, std records |
| llvm-sieve | xo | 1182 | 49 | 41 | 44 | 229 / 54 / 55 | 2.15 | separate, std records |
| llvm-nbody | xo | 1179 | 53 | 45 | 44 | 231 / 57 / 56 | 2.15 | separate, std records |
| llvm-sort | xo | 1286 | 51 | 42 | 43 | 225 / 57 / 55 | 2.15 | separate, std records |
| llvm-interp | xo | 1168 | 50 | 42 | 44 | 234 / 57 / 57 | 2.15 | separate, std records |
| llvm-collatz | xo | 1177 | 49 | 40 | 41 | 228 / 56 / 55 | 2.15 | separate, std records |
| llvm-strings | xo | 1168 | 48 | 41 | 41 | 230 / 57 / 55 | 2.15 | separate, std records |
| coll | xo | 1314 | 65 | 54 | 57 | 228 / 62 / 68 | 2.26 | separate, std records |
| conc-spawn | xo | 1360 | 54 | 46 | 46 | 233 / 58 / 58 | 2.24 | separate, std records |
| conc-fanout | xo | 1358 | 52 | 44 | 47 | 225 / 55 / 58 | 2.26 | separate, std records |
| conc-pingpong | xo | 1355 | 54 | 47 | 48 | 235 / 59 / 59 | 2.27 | separate, std records |
| conc-mutex | xo | 1434 | 53 | 47 | 48 | 235 / 59 / 58 | 2.31 | separate, std records |
| conc-rwmutex | xo | 1373 | 52 | 44 | 46 | 225 / 56 / 56 | 2.32 | separate, std records |
| conc-pmap | xo | 1353 | 53 | 44 | 45 | 233 / 55 / 58 | 2.27 | separate, std records |

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
B: /work/gatel2-rvgfix-f5ec0746b.logs/rel/xo-v0.48.1-linux-arm64/bin/xo test --std-module=base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.25/5.25 5.25/5.07 5.07/5.07

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1741        1397        0.669% 0.558%     0.802   0.8 to 0.802      0.792 0.81      better (n=3)
cpu_ms                9571        2366        0.47% 0.422%      0.247   0.245 to 0.249    0.247 0.248     better (n=3)
max_rss_kb            308532      322432      0.325% 0.066%     1.045   1.043 to 1.051    1.046 1.047     worse (n=3)
load: median 5.25 (A) / 5.07 (B); loaded: 0 of 3 rounds (at or above 8.63; baseline 5.63 + 3.0, at least 6.0)
```

### base64 (encoding/base64) edit (load 5 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/base64 
B: /work/gatel2-rvgfix-f5ec0746b.logs/rel/xo-v0.48.1-linux-arm64/bin/xo test --std-module=base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.91/5.91 5.91/5.91 5.91/5.91

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               246.1       88.21       0.261% 1.866%     0.358   0.35 to 0.363     0.358 0.358     better (n=3)
cpu_ms                718.1       90.61       0.446% 2.699%     0.126   0.123 to 0.131    0.126 0.126     better (n=3)
max_rss_kb            109108      101400      0.796% 0.625%     0.929   0.922 to 0.937    0.931 0.922     better (n=3)
load: median 5.91 (A) / 5.91 (B); loaded: 0 of 3 rounds (at or above 8.73; baseline 5.73 + 3.0, at least 6.0)
```

### json (encoding/json) cold (load 5 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/json 
B: /work/gatel2-rvgfix-f5ec0746b.logs/rel/xo-v0.48.1-linux-arm64/bin/xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.52/5.52 9.0/8.36 8.36/8.36

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               3537        1653        0.399% 0.251%     0.467   0.467 to 0.469    0.466 0.469     better (n=3)
cpu_ms                20890       2921        0.579% 0.228%     0.14    0.138 to 0.141    0.14 0.14       better (n=3)
max_rss_kb            302476      334740      0.992% 0.577%     1.107   1.099 to 1.129    1.111 1.105     worse (n=3)
load: median 8.36 (A) / 8.36 (B); loaded: 1 of 3 rounds (at or above 8.91; baseline 5.91 + 3.0, at least 6.0)
```

### json (encoding/json) edit (load 4 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/json 
B: /work/gatel2-rvgfix-f5ec0746b.logs/rel/xo-v0.48.1-linux-arm64/bin/xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.86/4.86 4.86/4.63 4.63/4.63

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               733.2       137.5       1.057% 8.512%     0.187   0.185 to 0.216    0.185 0.219     better (n=3)
cpu_ms                1879        172.1       0.243% 35.29%     0.092   0.09 to 0.155     0.091 0.155     better (n=3)
max_rss_kb            159832      122720      1.19% 0.095%      0.768   0.759 to 0.779    0.777 0.765     better (n=3)
load: median 4.86 (A) / 4.63 (B); loaded: 0 of 3 rounds (at or above 7.86; baseline 4.86 + 3.0, at least 6.0)
```

