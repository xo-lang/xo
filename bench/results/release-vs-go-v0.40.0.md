# Xo against Go: xo 0.40.0, the release (2026-11-05)

scripts/release-vs-go.sh with the released xo 0.40.0 (9bb970597):
- Compile and size, Run and Test: Darwin arm64, 24 GB, Go go1.26.1.
- HTTP section: the linux/arm64 offload host, against the 0.39.0 release.
- amd64 micro set: the linux/amd64 offload host, against 0.39.0 and Go.

The first run's measure job was stopped by heavyd's 30-minute stall rule
while its HTTP section waited for host 1's exclusive lease. The Mac
sections had already finished, and they are kept. The HTTP and amd64
sections were rerun on their own (rvg-final-b, no Mac slot). Two reports
are joined below unchanged: the first run's Mac sections, then the
rerun's HTTP and amd64 sections.

## Against the pre-cut run (release-vs-go-v0.40.0-precut.md)

- Run rows (B/A, Xo over Go): every row is better than the pre-cut or
  within noise, except LLVM nbody at 1.026, which was already 1.037 in
  the pre-cut. It is not a change between the pre-cut and the release.
- HTTP, xo 0.40.0 against 0.39.0: every row is "placement" in the pad
  sweep, with req/s medians from 0.996 to 1.000 and CPU per request from
  0.995 to 1.001. Against Go: /alloc c32 5.98x req/s, /hello c256 1.81x.

Note (2026-11-08): release-vs-go-amd64.sh ran these binaries from /mnt/c,
where drvfs adds about 6 ms of exec to every run (fib32 13.8 against 7.9 ms
from ext4), more for Go's larger binary, so the short rows' ratios against Go
read too favorable to Xo and small changes against the release before are
hidden. gotchas/wsl-drvfs-exec-cost.md; fixed in f26b951fb.

- amd64 micro set: no row is flagged (none is more than 3% slower than
  0.39.0). One memory note: json10kx20's max RSS is 1.047x 0.39.0's
  (43,096 to 45,100 KB) at the same wall time.
- Known issue found after the release (fixed on main in 6c2df7a26): a
  native HTTP server with idle keep-alive connections could hang on
  SIGTERM, in 0.36.0 to 0.40.0 (std/changes/http.md). Throughput rows
  are not affected.

# Xo against Go: xo 0.40.0 darwin/arm64 (2026-11-05)

scripts/release-vs-go.sh on Darwin arm64, 24 GB,
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
| llvm | go | 2020 | 50 | 40 | 130 | 312 / 26 / 67 | 2.39 | - |
| coll | go | 1690 | 40 | 50 | 130 | 308 / 26 / 68 | 2.43 | - |
| conc | go | 1430 | 30 | 40 | 100 | 299 / 26 / 69 | 2.40 | - |
| llvm-fib | xo | 1210 | 60 | 40 | 40 | 252 / 40 / 41 | 1.31 | separate, std records |
| llvm-sieve | xo | 1070 | 60 | 40 | 40 | 245 / 40 / 41 | 1.33 | separate, std records |
| llvm-nbody | xo | 1070 | 70 | 40 | 40 | 247 / 41 / 42 | 1.33 | separate, std records |
| llvm-sort | xo | 1060 | 60 | 40 | 40 | 246 / 40 / 42 | 1.33 | separate, std records |
| llvm-interp | xo | 1060 | 60 | 50 | 50 | 252 / 42 / 42 | 1.33 | separate, std records |
| llvm-collatz | xo | 1180 | 60 | 40 | 40 | 247 / 39 / 41 | 1.33 | separate, std records |
| llvm-strings | xo | 1100 | 60 | 40 | 40 | 246 / 40 / 42 | 1.33 | separate, std records |
| coll | xo | 1120 | 130 | 100 | 110 | 251 / 52 / 52 | 1.50 | separate, std records |
| conc-spawn | xo | 1230 | 110 | 50 | 70 | 256 / 41 / 41 | 1.41 | separate, std records |
| conc-fanout | xo | 1350 | 60 | 40 | 40 | 256 / 43 / 42 | 1.41 | separate, std records |
| conc-pingpong | xo | 1420 | 60 | 40 | 40 | 254 / 48 / 48 | 1.43 | separate, std records |
| conc-mutex | xo | 1250 | 60 | 40 | 40 | 258 / 42 / 42 | 1.46 | separate, std records |
| conc-rwmutex | xo | 1220 | 70 | 40 | 40 | 255 / 41 / 44 | 1.47 | separate, std records |
| conc-pmap | xo | 1230 | 60 | 40 | 40 | 256 / 41 / 42 | 1.41 | separate, std records |

The Go column builds one binary per group (llvm, coll, conc); the Xo
programs are one per workload except coll.

## Run (xo bench --ab, A = Go, B = Xo, 3 rounds)

### llvm fib (load 5 at start)

```
A: llvm.go fib 
B: llvm-fib.xo fib 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.38/5.38 5.38/5.38 5.38/5.38

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               229.8       225.1       2.829% 2.038%     0.98    0.972 to 0.987    better (n=3)
cpu_ms                229.1       224.3       3.091% 2.047%     0.979   0.971 to 0.991    better (n=3)
max_rss_kb            4160        2624        1.154% 0.305%     0.631   0.627 to 0.638    better (n=3)
load: median 5.38 (A) / 5.38 (B); loaded: 0 of 3 rounds (at or above 8.38; baseline 5.38 + 3.0, at least 6.0)
```

### llvm sieve (load 5 at start)

```
A: llvm.go sieve 
B: llvm-sieve.xo sieve 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.38/5.38 5.38/5.38 5.38/5.38

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               112.1       109.3       2.557% 4.693%     0.975   0.921 to 1.035    within noise (n=3)
cpu_ms                171.3       108.3       8.162% 4.169%     0.632   0.612 to 0.695    better (n=3)
max_rss_kb            45296       13264       4.875% 0.06%      0.293   0.279 to 0.307    better (n=3)
load: median 5.38 (A) / 5.38 (B); loaded: 0 of 3 rounds (at or above 8.38; baseline 5.38 + 3.0, at least 6.0)
```

### llvm nbody (load 5 at start)

```
A: llvm.go nbody 
B: llvm-nbody.xo nbody 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.03/5.03 5.03/5.03 5.03/5.03

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               162.6       166.8       0.52% 0.447%      1.026   1.023 to 1.027    worse (n=3)
cpu_ms                161.9       165.9       0.099% 0.113%     1.025   1.023 to 1.025    worse (n=3)
max_rss_kb            4256        2640        2.256% 0.303%     0.62    0.609 to 0.637    better (n=3)
load: median 5.03 (A) / 5.03 (B); loaded: 0 of 3 rounds (at or above 8.38; baseline 5.38 + 3.0, at least 6.0)
```

### llvm sort (load 5 at start)

```
A: llvm.go sort 
B: llvm-sort.xo sort 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.03/5.03 5.03/5.03 5.03/5.03

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               268.6       219.8       0.472% 0.146%     0.818   0.816 to 0.825    better (n=3)
cpu_ms                284.2       218.9       0.369% 0.18%      0.77    0.767 to 0.772    better (n=3)
max_rss_kb            75808       26768       26.84% 0%         0.353   0.264 to 0.442    better (n=3)
load: median 5.03 (A) / 5.03 (B); loaded: 0 of 3 rounds (at or above 8.03; baseline 5.03 + 3.0, at least 6.0)
```

### llvm interp (load 5 at start)

```
A: llvm.go interp 
B: llvm-interp.xo interp 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.03/5.03 5.03/5.03 5.03/5.03

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               147.3       42.17       0.884% 2.004%     0.286   0.282 to 0.289    better (n=3)
cpu_ms                146.6       41.4        0.822% 2.307%     0.282   0.277 to 0.286    better (n=3)
max_rss_kb            4368        2672        1.099% 0.898%     0.612   0.603 to 0.62     better (n=3)
load: median 5.03 (A) / 5.03 (B); loaded: 0 of 3 rounds (at or above 8.03; baseline 5.03 + 3.0, at least 6.0)
```

### llvm collatz (load 5 at start)

```
A: llvm.go collatz 
B: llvm-collatz.xo collatz 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.79/4.79 4.79/4.79 4.79/4.79

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               495.1       445.5       0.324% 0.393%     0.9     0.897 to 0.9      better (n=3)
cpu_ms                494.7       444.6       0.193% 0.396%     0.899   0.896 to 0.9      better (n=3)
max_rss_kb            4224        2624        1.326% 0%         0.621   0.619 to 0.636    better (n=3)
load: median 4.79 (A) / 4.79 (B); loaded: 0 of 3 rounds (at or above 8.03; baseline 5.03 + 3.0, at least 6.0)
```

### llvm strings (load 4 at start)

```
A: llvm.go strings 
B: llvm-strings.xo strings 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.79/4.79 4.79/4.79 4.79/4.79

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               90.2        45.06       2.652% 4.006%     0.5     0.5 to 0.515      better (n=3)
cpu_ms                126.3       44.15       4.442% 4.081%     0.35    0.35 to 0.36      better (n=3)
max_rss_kb            24704       18304       9.715% 0.044%     0.741   0.683 to 0.832    better (n=3)
load: median 4.79 (A) / 4.79 (B); loaded: 0 of 3 rounds (at or above 7.79; baseline 4.79 + 3.0, at least 6.0)
```

### coll putget (load 4 at start)

```
A: coll.go putget 
B: coll.xo putget 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.79/4.79 4.79/4.79 4.79/4.79

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               50.36       35.31       0.552% 1.654%     0.701   0.699 to 0.715    better (n=3)
cpu_ms                49.73       34.4        0.639% 1.686%     0.692   0.685 to 0.699    too coarse (n=3)
max_rss_kb            8800        5952        1.909% 0%         0.676   0.654 to 0.679    better (n=3)
load: median 4.79 (A) / 4.79 (B); loaded: 0 of 3 rounds (at or above 7.79; baseline 4.79 + 3.0, at least 6.0)
```

### coll get (load 4 at start)

```
A: coll.go get 
B: coll.xo get 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.79/4.79 4.79/4.79 4.79/4.79

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.86       13.68       1.509% 5.387%     0.599   0.586 to 0.661    better (n=3)
cpu_ms                22.09       12.86       1.714% 0.109%     0.582   0.567 to 0.586    too coarse (n=3)
max_rss_kb            7168        5952        0.781% 0.269%     0.83    0.83 to 0.848     better (n=3)
load: median 4.79 (A) / 4.79 (B); loaded: 0 of 3 rounds (at or above 7.79; baseline 4.79 + 3.0, at least 6.0)
```

### coll push (load 4 at start)

```
A: coll.go push 
B: coll.xo push 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.79/4.79 4.79/4.79 4.79/4.79

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               12.74       9.516       8.08% 9.379%      0.747   0.668 to 0.804    better (n=3)
cpu_ms                17.41       8.365       9.713% 5.415%     0.48    0.421 to 0.48     too coarse (n=3)
max_rss_kb            54112       26944       3.519% 0%         0.498   0.465 to 0.498    better (n=3)
load: median 4.79 (A) / 4.79 (B); loaded: 0 of 3 rounds (at or above 7.79; baseline 4.79 + 3.0, at least 6.0)
```

### coll update (load 4 at start)

```
A: coll.go update 
B: coll.xo update 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.79/4.79 4.79/4.79 4.48/4.79

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               31.1        19.96       2.096% 1.265%     0.642   0.622 to 0.665    better (n=3)
cpu_ms                30.75       19.07       1.424% 0.986%     0.62    0.614 to 0.644    too coarse (n=3)
max_rss_kb            8672        5952        2.214% 0%         0.686   0.681 to 0.713    better (n=3)
load: median 4.79 (A) / 4.79 (B); loaded: 0 of 3 rounds (at or above 7.79; baseline 4.79 + 3.0, at least 6.0)
```

### coll iter (load 4 at start)

```
A: coll.go iter 
B: coll.xo iter 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.48/4.48 4.48/4.48 4.48/4.48

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               21.95       5.787       1.032% 4.199%     0.264   0.26 to 0.278     better (n=3)
cpu_ms                21.21       4.933       0.929% 5.767%     0.233   0.231 to 0.254    too coarse (n=3)
max_rss_kb            7168        5952        1.004% 0%         0.83    0.827 to 0.844    better (n=3)
load: median 4.48 (A) / 4.48 (B); loaded: 0 of 3 rounds (at or above 7.48; baseline 4.48 + 3.0, at least 6.0)
```

### coll copymut (load 4 at start)

```
A: coll.go copymut 
B: coll.xo copymut 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.48/4.48 4.48/4.48 4.48/4.48

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               14.27       9.8         4.585% 0.939%     0.687   0.642 to 0.697    better (n=3)
cpu_ms                14.09       9.049       1.331% 0.796%     0.642   0.632 to 0.645    too coarse (n=3)
max_rss_kb            10256       5984        3.276% 0%         0.583   0.577 to 0.617    better (n=3)
load: median 4.48 (A) / 4.48 (B); loaded: 0 of 3 rounds (at or above 7.48; baseline 4.48 + 3.0, at least 6.0)
```

### coll branch (load 4 at start)

```
A: coll.go branch 
B: coll.xo branch 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.48/4.48 4.48/4.48 4.48/4.48

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               198.6       5.335       3.532% 4.508%     0.027   0.025 to 0.027    better (n=3)
cpu_ms                309.6       4.47        4.006% 4.228%     0.014   0.013 to 0.015    better (n=3)
max_rss_kb            16016       5952        6.793% 0%         0.372   0.348 to 0.399    better (n=3)
load: median 4.48 (A) / 4.48 (B); loaded: 0 of 3 rounds (at or above 7.48; baseline 4.48 + 3.0, at least 6.0)
```

### conc spawn (load 4 at start)

```
A: conc.go spawn 
B: conc-spawn.xo spawn 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.48/4.48 4.48/4.48 4.48/4.48

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               50.72       21.69       3.793% 0.509%     0.428   0.424 to 0.454    better (n=3)
cpu_ms                168.0       22.52       3.352% 3.695%     0.134   0.132 to 0.134    better (n=3)
max_rss_kb            11024       4336        5.08% 2.399%      0.393   0.367 to 0.396    better (n=3)
load: median 4.48 (A) / 4.48 (B); loaded: 0 of 3 rounds (at or above 7.48; baseline 4.48 + 3.0, at least 6.0)
```

### conc fanout (load 4 at start)

```
A: conc.go fanout 
B: conc-fanout.xo fanout 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.48/4.48 4.48/4.48 4.48/4.48

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               24.71       16.25       9.672% 4.175%     0.658   0.587 to 0.73     better (n=3)
cpu_ms                68.64       15.66       9.045% 4.623%     0.228   0.211 to 0.255    too coarse (n=3)
max_rss_kb            11952       4432        2.744% 3.069%     0.371   0.371 to 0.382    better (n=3)
load: median 4.48 (A) / 4.48 (B); loaded: 0 of 3 rounds (at or above 7.48; baseline 4.48 + 3.0, at least 6.0)
```

### conc pingpong (load 4 at start)

```
A: conc.go pingpong 
B: conc-pingpong.xo pingpong 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.48/4.48 4.48/4.48 4.48/4.48

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               19.62       17.97       1.3% 0.587%       0.916   0.907 to 0.936    better (n=3)
cpu_ms                26.42       17.13       1.794% 0.464%     0.648   0.642 to 0.672    too coarse (n=3)
max_rss_kb            4240        2720        0.943% 1.176%     0.642   0.642 to 0.657    better (n=3)
load: median 4.48 (A) / 4.48 (B); loaded: 0 of 3 rounds (at or above 7.48; baseline 4.48 + 3.0, at least 6.0)
```

### conc mutex (load 4 at start)

```
A: conc.go mutex 
B: conc-mutex.xo mutex 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.48/4.48 4.48/4.48 4.48/4.48

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.95       6.919       2.296% 12.62%     0.301   0.3 to 0.394      better (n=3)
cpu_ms                95.58       6.104       4.788% 13.47%     0.064   0.063 to 0.088    too coarse (n=3)
max_rss_kb            4640        2672        1.897% 0.299%     0.576   0.56 to 0.585     better (n=3)
load: median 4.48 (A) / 4.48 (B); loaded: 0 of 3 rounds (at or above 7.48; baseline 4.48 + 3.0, at least 6.0)
```

### conc rwmutex (load 4 at start)

```
A: conc.go rwmutex 
B: conc-rwmutex.xo rwmutex 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.48/4.48 4.48/4.48 4.48/4.48

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               10.97       9.018       11.25% 11.29%     0.822   0.815 to 0.949    better (n=3)
cpu_ms                15.46       8.273       7.496% 11.7%      0.535   0.527 to 0.598    too coarse (n=3)
max_rss_kb            4800        2704        2.5% 0%           0.563   0.556 to 0.585    better (n=3)
load: median 4.48 (A) / 4.48 (B); loaded: 0 of 3 rounds (at or above 7.48; baseline 4.48 + 3.0, at least 6.0)
```

### conc pmap (load 4 at start)

```
A: conc.go pmap 
B: conc-pmap.xo pmap 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.48/4.48 4.48/4.48 4.48/4.48

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               55.15       54.39       0.724% 0.768%     0.986   0.968 to 0.997    better (n=3)
cpu_ms                558.4       555.0       0.077% 0.409%     0.994   0.989 to 0.998    better (n=3)
max_rss_kb            6880        5456        1.86% 0.88%       0.793   0.768 to 0.811    better (n=3)
load: median 4.48 (A) / 4.48 (B); loaded: 0 of 3 rounds (at or above 7.48; baseline 4.48 + 3.0, at least 6.0)
```


## Test (xo bench --ab, A = go test, B = xo test --std-module, 3 rounds)

Cold empties every cache before each round (Go with -count=1); edit appends one
function to the module under test before each round (Go -count=1). Copies of GOROOT
and the release's std.

### base64 (encoding/base64) cold (load 4 at start)

```
A: goroot/bin/go test -count=1 encoding/base64 
B: ~/xo-wt/rvg040-rel/xo-v0.40.0-darwin-arm64/bin/xo test --std-module=native/base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 4.03/4.03 3.86/3.86 5.08/3.86

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               2251        1457        7.211% 7.396%     0.647   0.647 to 0.663    better (n=3)
cpu_ms                11647       2233        3.23% 5.586%      0.192   0.19 to 0.199     better (n=3)
max_rss_kb            323760      357264      2.75% 0.296%      1.103   1.09 to 1.16      worse (n=3)
load: median 4.03 (A) / 3.86 (B); loaded: 0 of 3 rounds (at or above 7.2; baseline 4.2 + 3.0, at least 6.0)
```

### base64 (encoding/base64) edit (load 5 at start)

```
A: goroot/bin/go test -count=1 encoding/base64 
B: ~/xo-wt/rvg040-rel/xo-v0.40.0-darwin-arm64/bin/xo test --std-module=native/base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 6.62/6.62 6.62/6.62 6.62/6.62

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               616.6       398.9       13.01% 2.044%     0.647   0.57 to 0.739     better (n=3)
cpu_ms                1139        457.1       3.387% 1.511%     0.401   0.388 to 0.429    better (n=3)
max_rss_kb            114640      208048      2.17% 0.815%      1.815   1.773 to 1.837    worse (n=3)
load: median 6.62 (A) / 6.62 (B); loaded: 0 of 3 rounds (at or above 8.63; baseline 5.63 + 3.0, at least 6.0)
```

### json (encoding/json) cold (load 13 at start)

```
A: goroot/bin/go test -count=1 encoding/json 
B: ~/xo-wt/rvg040-rel/xo-v0.40.0-darwin-arm64/bin/xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 17.21/18.18 17.19/16.29 16.29/16.29

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               4527        1807        1.236% 2.523%     0.399   0.384 to 0.414    better (n=3)
cpu_ms                23739       2912        0.579% 2.57%      0.123   0.119 to 0.124    better (n=3)
max_rss_kb            315536      380448      2.216% 0.055%     1.206   1.204 to 1.26     worse (n=3)
load: median 17.19 (A) / 16.29 (B); loaded: 2 of 3 rounds (at or above 16.58; baseline 13.58 + 3.0, at least 6.0)
warning: the machine was busy through the run (median load at or above 16.58): wall and cpu ratios are not reliable; measure with HEAVY_CLASS=measure (agent-tools.md Measuring) or --max-load
```

### json (encoding/json) edit (load 5 at start)

```
A: goroot/bin/go test -count=1 encoding/json 
B: ~/xo-wt/rvg040-rel/xo-v0.40.0-darwin-arm64/bin/xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.63/5.63 5.63/5.42 5.42/5.42

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               1028        354.9       0.241% 0.803%     0.345   0.34 to 0.347     better (n=3)
cpu_ms                2481        369.1       3.21% 2.879%      0.149   0.147 to 0.149    better (n=3)
max_rss_kb            162832      224096      0.103% 0.846%     1.376   1.369 to 1.389    worse (n=3)
load: median 5.63 (A) / 5.42 (B); loaded: 0 of 3 rounds (at or above 8.63; baseline 5.63 + 3.0, at least 6.0)
```


## HTTP server (bench/perf/jitter, linux/arm64 offload host)

- host load 2.13 2.49 3.01 1/382 175785
- release-vs-go-http rvg37602: 9bb9705972 on Linux aarch64, 12 CPUs, go1.26.1
- xo xo 0.40.0 linux/arm64; base xo 0.39.0 linux/arm64
- loadgen flags: -c C -rate R -d 10s -warmup 2s -awake (default on in open loop); servers on CPUs 0-5, loadgen on 6-11
- Go's /alloc rows are its /alloc-hand (json.Marshal written by hand, as Xo's derive encodes).
- Built at each XO_ARM64_PAD of HTTP_PADS: the table pools every pad's rounds; the pad sweep verdict below judges xo against base.
- Rounds: 12 per row; each cell is the median, its range in brackets; CPU is the server's CPU microseconds per request.

| row | server | req/s | p50 us | p99 us | p99.9 us | max us | CPU us/req | errors |
|---|---|---|---|---|---|---|---|---|
| /alloc c32 | xo | 167,460 [163,958 to 170,085] | 175 [172 to 178] | 457 [452 to 469] | 1,256 [1,216 to 1,482] | 6,320 [6,139 to 13,760] | 34.4 [33.9 to 35.5] | 0 [0 to 0] |
| /alloc c32 | base | 168,094 [166,247 to 170,659] | 174 [172 to 176] | 458 [452 to 466] | 1,264 [1,217 to 1,373] | 6,324 [6,108 to 9,812] | 34.4 [33.7 to 34.9] | 0 [0 to 0] |
| /alloc c32 | go | 28,022 [27,880 to 28,129] | 488 [481 to 491] | 5,518 [5,452 to 5,538] | 8,065 [7,988 to 8,166] | 14,758 [13,274 to 14,833] | 118.8 [116.9 to 119.7] | 0 [0 to 0] |
| /alloc c64 open 10000/s | xo | 10,000 [10,000 to 10,000] | 78 [78 to 79] | 114 [109 to 125] | 142 [134 to 161] | 212 [188 to 734] | 52.0 [51.3 to 53.1] | 0 [0 to 0] |
| /alloc c64 open 10000/s | base | 10,000 [10,000 to 10,000] | 79 [78 to 80] | 114 [109 to 122] | 142 [137 to 153] | 505 [194 to 2,524] | 52.3 [51.3 to 54.3] | 0 [0 to 0] |
| /alloc c64 open 10000/s | go | 10,000 [10,000 to 10,000] | 100 [100 to 101] | 391 [383 to 395] | 514 [492 to 547] | 898 [794 to 5,651] | 122.7 [121.4 to 123.5] | 0 [0 to 0] |
| /alloc c64 open 20000/s | xo | 20,000 [20,000 to 20,000] | 91 [91 to 92] | 144 [143 to 145] | 167 [166 to 169] | 231 [211 to 470] | 57.6 [57.4 to 58.5] | 0 [0 to 0] |
| /alloc c64 open 20000/s | base | 20,000 [20,000 to 20,000] | 91 [91 to 92] | 144 [142 to 151] | 168 [166 to 184] | 256 [209 to 389] | 57.8 [57.3 to 58.8] | 0 [0 to 0] |
| /alloc c64 open 20000/s | go | 20,000 [20,000 to 20,000] | 189 [188 to 190] | 674 [660 to 769] | 979 [923 to 9,533] | 3,593 [2,569 to 18,480] | 127.3 [126.6 to 127.6] | 0 [0 to 0] |
| /alloc c256 | xo | 178,490 [174,731 to 181,454] | 1,248 [1,229 to 1,280] | 5,201 [5,111 to 5,304] | 6,528 [6,355 to 6,885] | 10,356 [9,604 to 18,262] | 33.6 [33.0 to 34.3] | 0 [0 to 0] |
| /alloc c256 | base | 178,576 [159,846 to 181,708] | 1,249 [1,225 to 1,349] | 5,184 [5,100 to 5,791] | 6,472 [6,388 to 8,373] | 10,449 [10,108 to 17,705] | 33.6 [33.0 to 37.4] | 0 [0 to 0] |
| /alloc c256 | go | 56,979 [56,573 to 57,062] | 393 [389 to 397] | 38,957 [38,583 to 39,126] | 58,415 [57,526 to 58,555] | 119,143 [115,808 to 120,474] | 82.2 [82.0 to 82.7] | 0 [0 to 0] |
| /hello c32 | xo | 353,878 [350,324 to 358,394] | 87 [86 to 88] | 221 [218 to 224] | 322 [310 to 337] | 3,731 [3,277 to 4,921] | 7.4 [7.3 to 7.4] | 0 [0 to 0] |
| /hello c32 | base | 354,080 [351,385 to 356,511] | 87 [86 to 88] | 222 [220 to 225] | 324 [317 to 336] | 3,955 [3,258 to 10,218] | 7.4 [7.3 to 7.4] | 0 [0 to 0] |
| /hello c32 | go | 275,238 [274,141 to 276,827] | 103 [102 to 103] | 628 [626 to 636] | 839 [835 to 851] | 4,265 [3,574 to 10,147] | 12.3 [12.3 to 12.4] | 0 [0 to 0] |
| /hello c256 | xo | 882,563 [876,353 to 891,139] | 259 [257 to 261] | 832 [810 to 851] | 1,672 [1,618 to 1,796] | 8,330 [6,217 to 10,543] | 5.9 [5.9 to 6.0] | 0 [0 to 0] |
| /hello c256 | base | 882,304 [871,485 to 890,737] | 260 [257 to 261] | 830 [807 to 857] | 1,696 [1,601 to 1,780] | 8,460 [6,374 to 12,023] | 5.9 [5.9 to 6.0] | 0 [0 to 0] |
| /hello c256 | go | 487,492 [483,548 to 491,045] | 307 [307 to 309] | 2,640 [2,587 to 2,666] | 4,574 [4,189 to 4,751] | 8,003 [7,987 to 11,083] | 10.0 [9.9 to 10.0] | 0 [0 to 0] |

- /alloc c32, xo/go medians: req/s 5.976, p50 us 0.359, p99 us 0.083, p99.9 us 0.156, max us 0.428, CPU us/req 0.289
- /alloc c32, xo/base medians: req/s 0.996, p50 us 1.003, p99 us 0.997, p99.9 us 0.994, max us 0.999, CPU us/req 1.001
- /alloc c64 open 10000/s, xo/go medians: req/s 1.000, p50 us 0.780, p99 us 0.290, p99.9 us 0.277, max us 0.236, CPU us/req 0.424
- /alloc c64 open 10000/s, xo/base medians: req/s 1.000, p50 us 0.987, p99 us 0.991, p99.9 us 1.007, max us 0.420, CPU us/req 0.995
- /alloc c64 open 20000/s, xo/go medians: req/s 1.000, p50 us 0.481, p99 us 0.213, p99.9 us 0.171, max us 0.064, CPU us/req 0.452
- /alloc c64 open 20000/s, xo/base medians: req/s 1.000, p50 us 1.000, p99 us 1.000, p99.9 us 0.997, max us 0.901, CPU us/req 0.997
- /alloc c256, xo/go medians: req/s 3.133, p50 us 3.177, p99 us 0.134, p99.9 us 0.112, max us 0.087, CPU us/req 0.409
- /alloc c256, xo/base medians: req/s 1.000, p50 us 1.000, p99 us 1.003, p99.9 us 1.009, max us 0.991, CPU us/req 1.001
- /hello c32, xo/go medians: req/s 1.286, p50 us 0.845, p99 us 0.352, p99.9 us 0.383, max us 0.875, CPU us/req 0.597
- /hello c32, xo/base medians: req/s 0.999, p50 us 1.000, p99 us 0.998, p99.9 us 0.992, max us 0.943, CPU us/req 0.997
- /hello c256, xo/go medians: req/s 1.810, p50 us 0.844, p99 us 0.315, p99.9 us 0.366, max us 1.041, CPU us/req 0.595
- /hello c256, xo/base medians: req/s 1.000, p50 us 0.998, p99 us 1.002, p99.9 us 0.986, max us 0.985, CPU us/req 1.000

pad sweep verdict (reference base): each cell is the median over pads of each pad's median over rounds, the pad medians' range in brackets; slower or faster only when the ranges do not overlap

| row | group | pads | req/s | CPU us/req | req/s vs ref | verdict |
|---|---|---|---|---|---|---|
| /alloc c32 | base | 4 | 168,083 [166,815 to 170,351] | 34.27 [33.95 to 34.86] |  |  |
| /alloc c32 | go | 1 | 28,022 [28,022 to 28,022] | 118.83 [118.83 to 118.83] | 0.167 | one pad: not judged |
| /alloc c32 | xo | 4 | 167,460 [164,459 to 168,995] | 34.37 [34.00 to 35.47] | 0.996 | placement |
| /alloc c64 open 10000/s | base | 4 | 10,000 [10,000 to 10,000] | 52.30 [52.10 to 52.30] |  |  |
| /alloc c64 open 10000/s | go | 1 | 10,000 [10,000 to 10,000] | 122.70 [122.70 to 122.70] | 1.000 | one pad: not judged |
| /alloc c64 open 10000/s | xo | 4 | 10,000 [10,000 to 10,000] | 52.15 [51.40 to 52.80] | 1.000 | placement |
| /alloc c64 open 20000/s | base | 4 | 20,000 [20,000 to 20,000] | 57.80 [57.35 to 58.30] |  |  |
| /alloc c64 open 20000/s | go | 1 | 20,000 [20,000 to 20,000] | 127.35 [127.35 to 127.35] | 1.000 | one pad: not judged |
| /alloc c64 open 20000/s | xo | 4 | 20,000 [20,000 to 20,000] | 57.55 [57.40 to 58.50] | 1.000 | placement |
| /alloc c256 | base | 4 | 178,518 [177,934 to 181,425] | 33.57 [33.02 to 33.67] |  |  |
| /alloc c256 | go | 1 | 56,979 [56,979 to 56,979] | 82.19 [82.19 to 82.19] | 0.319 | one pad: not judged |
| /alloc c256 | xo | 4 | 178,490 [174,881 to 180,173] | 33.58 [33.26 to 34.25] | 1.000 | placement |
| /hello c32 | base | 4 | 354,274 [353,490 to 355,443] | 7.36 [7.35 to 7.38] |  |  |
| /hello c32 | go | 1 | 275,238 [275,238 to 275,238] | 12.31 [12.31 to 12.31] | 0.777 | one pad: not judged |
| /hello c32 | xo | 4 | 353,783 [352,515 to 354,960] | 7.36 [7.34 to 7.38] | 0.999 | placement |
| /hello c256 | base | 4 | 882,040 [880,895 to 888,796] | 5.94 [5.92 to 5.94] |  |  |
| /hello c256 | go | 1 | 487,492 [487,492 to 487,492] | 9.98 [9.98 to 9.98] | 0.553 | one pad: not judged |
| /hello c256 | xo | 4 | 882,414 [881,297 to 883,753] | 5.94 [5.93 to 5.95] | 1.000 | placement |

## linux/amd64 micro set (bench/perf/micro13, the linux/amd64 offload host)

- base: xo 0.39.0 darwin/arm64; new: xo 0.40.0 darwin/arm64; Go go1.26.1; all built for linux/amd64 (xo with --release, as the owner's tests/13-perf)
- each block is xo bench --ab, A = base or go, B = new; ratio is new over A; a row more than 3% slower than base is FLAGGED

```
host Linux x86_64, 28 CPUs, load 0.00
### fib32: base against new
A: rvg-micro13.base fib32 
B: rvg-micro13.new fib32 
5 of 5 rounds used (0 loaded), loads A/B per round: 0/0 0/0 0/0 0/0 0/0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               13.7        13.67       0.671% 4.022%     0.998   0.954 to 1.054    within noise (n=5)
cpu_ms                6.568       6.312       12.41% 16.9%      0.961   0.771 to 1.267    too coarse (n=5)
max_rss_kb            1196        1196        0% 0.334%         1.0     1.0 to 1.003      within noise (n=5)
load: median 0 (A) / 0 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0 + 3.0, at least 6.0)
### fib32: go against new
A: rvg-micro13.go fib32 
B: rvg-micro13.new fib32 
5 of 5 rounds used (0 loaded), loads A/B per round: 0/0 0/0 0/0 0/0 0/0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.72       13.6        5.86% 0.49%       0.599   0.567 to 0.638    better (n=5)
cpu_ms                13.43       7.045       6.588% 20.77%     0.524   0.418 to 0.557    too coarse (n=5)
max_rss_kb            2388        992.0       0% 0.403%         0.415   0.415 to 0.417    better (n=5)
load: median 0 (A) / 0 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0 + 3.0, at least 6.0)
### map1M: base against new
A: rvg-micro13.base map1M 
B: rvg-micro13.new map1M 
5 of 5 rounds used (0 loaded), loads A/B per round: 0/0 0/0 0/0 0/0 0/0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               90.26       80.23       6.189% 4.948%     0.889   0.815 to 1.017    within noise (n=5)
cpu_ms                85.14       73.4        7.43% 6.166%      0.862   0.79 to 1.017     too coarse (n=5)
max_rss_kb            73476       73248       0.305% 0.306%     0.997   0.997 to 1.0      better (n=5)
load: median 0 (A) / 0 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0 + 3.0, at least 6.0)
### map1M: go against new
A: rvg-micro13.go map1M 
B: rvg-micro13.new map1M 
5 of 5 rounds used (0 loaded), loads A/B per round: 0/0 0.24/0.24 0.24/0.24 0.24/0.24 0.24/0.24

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               109.9       85.74       0.176% 2.363%     0.78    0.729 to 0.819    better (n=5)
cpu_ms                107.7       78.06       1.14% 2.347%      0.725   0.688 to 0.781    better (n=5)
max_rss_kb            51444       73472       0.871% 0%         1.428   1.41 to 1.46      worse (n=5)
load: median 0.24 (A) / 0.24 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0 + 3.0, at least 6.0)
### strbuild1M: base against new
A: rvg-micro13.base strbuild1M 
B: rvg-micro13.new strbuild1M 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.24/0.24 0.24/0.24 0.24/0.24 0.24/0.24 0.24/0.24

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               17.89       17.82       0.79% 2.35%       0.996   0.91 to 1.006     within noise (n=5)
cpu_ms                11.12       11.47       11.75% 18.0%      1.031   0.766 to 1.236    too coarse (n=5)
max_rss_kb            12324       12096       0% 1.852%         0.981   0.981 to 1.018    within noise (n=5)
load: median 0.24 (A) / 0.24 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.24 + 3.0, at least 6.0)
### strbuild1M: go against new
A: rvg-micro13.go strbuild1M 
B: rvg-micro13.new strbuild1M 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.24/0.24 0.24/0.24 0.24/0.24 0.24/0.24 0.24/0.24

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               31.17       17.79       46.91% 2.713%     0.571   0.402 to 0.607    better (n=5)
cpu_ms                37.71       11.43       20.44% 11.05%     0.303   0.188 to 0.347    too coarse (n=5)
max_rss_kb            33652       12096       14.03% 1.852%     0.359   0.356 to 0.455    better (n=5)
load: median 0.24 (A) / 0.24 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.24 + 3.0, at least 6.0)
### sort1M: base against new
A: rvg-micro13.base sort1M 
B: rvg-micro13.new sort1M 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.24/0.24 0.24/0.24 0.24/0.24 0.24/0.24 0.24/0.24

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               107.2       69.01       0.207% 0.386%     0.643   0.633 to 0.651    better (n=5)
cpu_ms                100.8       61.63       0.684% 1.879%     0.612   0.607 to 0.629    better (n=5)
max_rss_kb            17924       8288        1.25% 0%          0.462   0.456 to 0.468    better (n=5)
load: median 0.24 (A) / 0.24 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.24 + 3.0, at least 6.0)
### sort1M: go against new
A: rvg-micro13.go sort1M 
B: rvg-micro13.new sort1M 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.24/0.24 0.24/0.24 0.24/0.24 0.24/0.24 0.24/0.24

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               79.92       68.89       1.332% 0.744%     0.862   0.85 to 0.99      better (n=5)
cpu_ms                71.89       61.7        2.145% 5.9%       0.858   0.839 to 0.891    too coarse (n=5)
max_rss_kb            10452       8288        0% 0%             0.793   0.761 to 0.793    better (n=5)
load: median 0.24 (A) / 0.24 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.24 + 3.0, at least 6.0)
### sort1M_copy: base against new
A: rvg-micro13.base sort1M_copy 
B: rvg-micro13.new sort1M_copy 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.24/0.24 0.24/0.24 0.24/0.24 0.24/0.24 0.24/0.24

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               74.83       69.19       1.795% 0.266%     0.925   0.878 to 0.929    better (n=5)
cpu_ms                68.27       61.32       1.79% 2.265%      0.898   0.846 to 0.952    too coarse (n=5)
max_rss_kb            18552       8064        1.207% 2.778%     0.435   0.435 to 0.452    better (n=5)
load: median 0.24 (A) / 0.24 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.24 + 3.0, at least 6.0)
### sort1M_copy: go against new
A: rvg-micro13.go sort1M_copy 
B: rvg-micro13.new sort1M_copy 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.24/0.24 0.24/0.24 0.24/0.24 0.24/0.3 0.3/0.3

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               80.78       68.75       1.521% 0.105%     0.851   0.844 to 1.016    within noise (n=5)
cpu_ms                72.79       62.51       2.932% 2.269%     0.859   0.787 to 0.93     too coarse (n=5)
max_rss_kb            10452       8288        2.105% 0%         0.793   0.772 to 0.793    better (n=5)
load: median 0.24 (A) / 0.24 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.24 + 3.0, at least 6.0)
### json10kx20: base against new
A: rvg-micro13.base json10kx20 
B: rvg-micro13.new json10kx20 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.3/0.3 0.3/0.3 0.3/0.3 0.3/0.3 0.3/0.3

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               78.77       77.7        1.642% 0.638%     0.986   0.982 to 1.185    within noise (n=5)
cpu_ms                70.31       70.31       1.631% 2.761%     1.0     0.96 to 1.194     too coarse (n=5)
max_rss_kb            43096       45100       0.334% 0.497%     1.047   1.041 to 1.052    worse (n=5)
load: median 0.3 (A) / 0.3 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.3 + 3.0, at least 6.0)
### json10kx20: go against new
A: rvg-micro13.go json10kx20 
B: rvg-micro13.new json10kx20 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.3/0.3 0.3/0.3 0.3/0.3 0.3/0.3 0.3/0.3

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               190.4       77.3        0.733% 1.549%     0.406   0.401 to 0.415    better (n=5)
cpu_ms                232.3       71.21       0.823% 0.317%     0.307   0.297 to 0.317    better (n=5)
max_rss_kb            13896       45100       5.815% 0%         3.246   3.064 to 3.362    worse (n=5)
load: median 0.3 (A) / 0.3 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.3 + 3.0, at least 6.0)
### spawn100k: base against new
A: rvg-micro13.base spawn100k 
B: rvg-micro13.new spawn100k 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.3/0.3 0.3/0.3 0.3/0.3 0.3/0.3 0.3/0.3

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               18.01       17.26       2.68% 3.555%      0.958   0.935 to 1.077    within noise (n=5)
cpu_ms                62.64       63.03       1.775% 6.726%     1.006   0.9 to 1.055      too coarse (n=5)
max_rss_kb            5336        5556        5.622% 26.21%     1.041   0.973 to 1.398    within noise (n=5)
load: median 0.3 (A) / 0.3 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.3 + 3.0, at least 6.0)
### spawn100k: go against new
A: rvg-micro13.go spawn100k 
B: rvg-micro13.new spawn100k 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.3/0.3 0.3/0.3 0.3/0.3 0.3/0.3 0.3/0.3

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               16.4        16.43       1.443% 2.48%      1.002   0.985 to 1.034    within noise (n=5)
cpu_ms                39.03       55.49       4.176% 5.707%     1.422   1.329 to 1.681    too coarse (n=5)
max_rss_kb            8212        5476        2.728% 4.894%     0.667   0.583 to 0.822    better (n=5)
load: median 0.3 (A) / 0.3 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.3 + 3.0, at least 6.0)
OFFLOAD-RC=0
```
