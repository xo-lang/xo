# Xo against Go: xo 0.50.1 linux/arm64 (2026-11-13)

The published v0.50.1 archive (xo-v0.50.1-linux-arm64.tar.gz, sha256 dfc7aae8..., offload host 1; tag 6d0a9dc7c), run by the script as of the commit that adds this
report. Since this report the headline table runs both tools with their default caches (owner, 2026-11-13);
the run-cache-off rows are a diagnostic table below it.

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
The compile table runs both tools with their default caches (owner, 2026-11-13): xo's
run cache on, Go as is; cold empties both. Reports from 2026-10-29 to 2026-11-12 ran xo
with the run cache off (XO_BIN_CACHE=off) in the headline table, so their second and warm
rows were xo's compile path against Go's up-to-date check; that table is now the
"xo compile path (run cache off)" diagnostic below it.
Size is the default build's binary.

## Compile and size

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB | edit's path |
|---|---|---|---|---|---|---|---|---|
| llvm | go | 1362 | 19 | 19 | 73 | 301 / 23 / 72 | 2.29 | - |
| coll | go | 1300 | 19 | 20 | 79 | 313 / 21 / 68 | 2.30 | - |
| conc | go | 1315 | 20 | 19 | 70 | 303 / 21 / 70 | 2.30 | - |
| llvm-fib | xo | 1167 | 52 | 8 | 51 | 227 / 12 / 56 | 2.14 | separate, std records |
| llvm-sieve | xo | 1173 | 51 | 6 | 53 | 229 / 12 / 56 | 2.15 | separate, std records |
| llvm-nbody | xo | 1162 | 68 | 6 | 61 | 229 / 12 / 61 | 2.15 | separate, std records |
| llvm-sort | xo | 1150 | 57 | 6 | 60 | 228 / 12 / 57 | 2.15 | separate, std records |
| llvm-interp | xo | 1166 | 69 | 13 | 64 | 230 / 12 / 60 | 2.15 | separate, std records |
| llvm-collatz | xo | 1153 | 56 | 5 | 49 | 236 / 12 / 57 | 2.15 | separate, std records |
| llvm-strings | xo | 1169 | 55 | 6 | 54 | 229 / 12 / 57 | 2.15 | separate, std records |
| coll | xo | 1203 | 74 | 11 | 67 | 233 / 12 / 66 | 2.25 | separate, std records |
| conc-spawn | xo | 1341 | 53 | 6 | 53 | 229 / 12 / 60 | 2.25 | separate, std records |
| conc-fanout | xo | 1376 | 60 | 12 | 60 | 230 / 12 / 60 | 2.26 | separate, std records |
| conc-pingpong | xo | 1343 | 61 | 6 | 54 | 231 / 12 / 63 | 2.28 | separate, std records |
| conc-mutex | xo | 1325 | 56 | 6 | 55 | 232 / 12 / 60 | 2.31 | separate, std records |
| conc-rwmutex | xo | 1336 | 82 | 11 | 75 | 228 / 12 / 60 | 2.31 | separate, std records |
| conc-pmap | xo | 1342 | 66 | 18 | 72 | 231 / 12 / 60 | 2.26 | separate, std records |

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
| llvm-fib | xo | 1106 | 70 | 74 | 64 | 231 / 59 / 56 | 2.14 | separate, std records |
| llvm-sieve | xo | 1104 | 70 | 81 | 61 | 227 / 58 / 56 | 2.15 | separate, std records |
| llvm-nbody | xo | 1139 | 72 | 58 | 50 | 223 / 58 / 56 | 2.15 | separate, std records |
| llvm-sort | xo | 1104 | 59 | 49 | 49 | 225 / 57 / 59 | 2.15 | separate, std records |
| llvm-interp | xo | 1116 | 50 | 47 | 57 | 226 / 58 / 59 | 2.15 | separate, std records |
| llvm-collatz | xo | 1102 | 52 | 64 | 65 | 228 / 58 / 56 | 2.15 | separate, std records |
| llvm-strings | xo | 1113 | 73 | 57 | 54 | 235 / 58 / 56 | 2.15 | separate, std records |
| coll | xo | 1161 | 92 | 92 | 91 | 228 / 65 / 66 | 2.25 | separate, std records |
| conc-spawn | xo | 1287 | 67 | 79 | 78 | 223 / 59 / 60 | 2.25 | separate, std records |
| conc-fanout | xo | 1308 | 52 | 53 | 58 | 230 / 59 / 60 | 2.26 | separate, std records |
| conc-pingpong | xo | 1282 | 59 | 85 | 80 | 220 / 63 / 63 | 2.28 | separate, std records |
| conc-mutex | xo | 1302 | 52 | 72 | 98 | 219 / 60 / 60 | 2.31 | separate, std records |
| conc-rwmutex | xo | 1292 | 53 | 55 | 52 | 228 / 60 / 60 | 2.31 | separate, std records |
| conc-pmap | xo | 1307 | 54 | 53 | 65 | 234 / 58 / 59 | 2.26 | separate, std records |

### Where xo's warm memory goes (coll, run cache off)

- the bare process (`xo version`): 7.6 MB peak RSS
- the warm build: 64.3 MB peak RSS; with the run cache on (its own directory, a hit) 12.7 MB

The warm build's phases (heap in use and the peak so far, --phase-times), the steps that move them:

```
phase check.setup 4.28 ms (heap 26 MB, peak 26 MB)
phase fe.build 9.59 ms (heap 45 MB, peak 51 MB)
cache fe 17/23 bodies
cache stdx used
phase front_end_freed 2.80 ms (heap 31 MB, peak 51 MB)
phase inline.apply 1.45 ms (heap 35 MB, peak 51 MB)
phase fn_keys 0.67 ms (heap 36 MB, peak 51 MB)
phase emit 2.68 ms (heap 33 MB, peak 51 MB)
cache functions 43/298 reused
cache parts 1/1 reused
phase link.key 0.00 ms (heap 32 MB, peak 51 MB)
phase link.store_prepare 2.84 ms (heap 44 MB, peak 51 MB)
phase link.layout 2.43 ms (heap 48 MB, peak 51 MB)
phase link.write 2.18 ms (heap 42 MB, peak 52 MB)
phase link.store_bin 0.00 ms (heap 40 MB, peak 52 MB)
phase link 10.71 ms (heap 40 MB, peak 52 MB)
```

The heap in use at the front end's peak (XO_PROF=heap at fe.build, every byte sampled), the top 12:

```
xo heap profile: 84940 samples at one per 1B allocated (estimated totals)
in use: 31.40MB in 75789 objects; allocated: 66.68MB in 195520 objects
      flat  flat%   sum%        cum   cum%  line (the largest)  function
    7.35MB 23.41% 23.41%     7.35MB 23.41%  load.xo:626         check.Checker.own_objs
    6.98MB 22.23% 45.65%     9.38MB 29.87%  export.xo:792       check.decode_std_export
    4.77MB 15.18% 60.82%     4.77MB 15.18%  raw.xo:32           raw.bytes_buf
    1.65MB  5.24% 66.07%     1.98MB  6.31%  -                   xo.map.put[Map[Str, StdFact]]
    1.57MB  5.01% 71.08%     2.25MB  7.18%  export.xo:505       check.ExR.obj
  895.45kB  2.79% 73.86%   895.45kB  2.79%  -                   xo.map.rebuildk[Str]
  799.83kB  2.49% 76.35%   799.83kB  2.49%  export.xo:265       check.ExR.ints
  644.55kB  2.00% 78.36%   856.71kB  2.66%  parser.xo:1727      parser.Parser.parse_type
  523.72kB  1.63% 79.99%   523.72kB  1.63%  stdfacts.xo:184     ssa.SfR.bools
  475.92kB  1.48% 81.47%   740.95kB  2.30%  rccache.xo:991      ssa.decode_func_at
  395.59kB  1.23% 82.70%   395.59kB  1.23%  export.xo:1241      check.Checker.relocate_export
  352.05kB  1.10% 83.79%   941.44kB  2.93%  export.xo:1499      check.Checker.install_std
```

## Test (xo bench --ab, A = go test, B = xo test --std-module, 3 rounds)

Cold empties every cache before each round (Go with -count=1); edit appends one
function to the module under test before each round (Go -count=1). Copies of GOROOT
and the release's std.

### base64 (encoding/base64) cold (load 5 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/base64 
B: /work/l2-rvg0501/xo/xo-v0.50.1-linux-arm64/bin/xo test --std-module=base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.09/5.09 4.76/4.76 4.46/4.76

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               1735        1330        0.769% 0.37%      0.767   0.757 to 0.772    0.769 0.76      better (n=3)
cpu_ms                9726        2223        0.908% 0.779%     0.229   0.229 to 0.231    0.234 0.227     better (n=3)
max_rss_kb            300392      328236      2.925% 0.525%     1.093   1.051 to 1.121    1.096 1.077     worse (n=3)
load: median 4.76 (A) / 4.76 (B); loaded: 0 of 3 rounds (at or above 8.09; baseline 5.09 + 3.0, at least 6.0)
```

### base64 (encoding/base64) edit (load 4 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/base64 
B: /work/l2-rvg0501/xo/xo-v0.50.1-linux-arm64/bin/xo test --std-module=base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.86/4.86 4.86/4.86 4.86/4.86

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               245.7       91.97       0.626% 4.191%     0.374   0.373 to 0.401    0.384 0.376     better (n=3)
cpu_ms                716.3       94.91       1.51% 7.812%      0.132   0.132 to 0.15     0.142 0.132     better (n=3)
max_rss_kb            110624      110988      1.036% 1.989%     1.003   0.961 to 1.022    1.001 1.002     within noise (n=3)
load: median 4.86 (A) / 4.86 (B); loaded: 0 of 3 rounds (at or above 7.86; baseline 4.86 + 3.0, at least 6.0)
```

### json (encoding/json) cold (load 4 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/json 
B: /work/l2-rvg0501/xo/xo-v0.50.1-linux-arm64/bin/xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.55/4.55 4.27/6.97 6.97/6.97

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               3546        1603        0.349% 0.472%     0.452   0.451 to 0.453    0.451 0.454     better (n=3)
cpu_ms                20709       2821        0.036% 0.886%     0.136   0.134 to 0.137    0.135 0.137     better (n=3)
max_rss_kb            302668      343652      2.812% 0.565%     1.135   1.076 to 1.144    1.086 1.132     worse (n=3)
load: median 4.55 (A) / 6.97 (B); loaded: 0 of 3 rounds (at or above 7.86; baseline 4.86 + 3.0, at least 6.0)
```

### json (encoding/json) edit (load 5 at start)

```
xo bench: warning: both sides build through one back-end cache, which slows the second build of each round (gotchas/bench-run-position-effect.md); give each side its own: --env-a=XO_BACKEND_CACHE_DIR=DIR/a --env-b=XO_BACKEND_CACHE_DIR=DIR/b
A: goroot/bin/go test -count=1 encoding/json 
B: /work/l2-rvg0501/xo/xo-v0.50.1-linux-arm64/bin/xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.27/5.27 5.27/5.01 5.01/5.01

metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               725.2       151.5       1.119% 2.373%     0.209   0.208 to 0.216    0.214 0.209     better (n=3)
cpu_ms                1858        195.4       1.464% 4.946%     0.105   0.105 to 0.113    0.109 0.106     better (n=3)
max_rss_kb            160124      147408      1.309% 0.947%     0.921   0.904 to 0.923    0.923 0.899     better (n=3)
load: median 5.27 (A) / 5.01 (B); loaded: 0 of 3 rounds (at or above 8.27; baseline 5.27 + 3.0, at least 6.0)
```

