# Xo against Go: main before the 0.39.0 cut (2026-11-04)

A pre-cut run of scripts/release-vs-go.sh (the lead, 2026-11-04) on main
79bee8d83 (Lang 2's map fix on main) with that commit's xo, Darwin arm64,
24 GB, Go go1.26.1; the HTTP section on the linux/arm64 offload host against
the 0.38.1 release, the amd64 section (6cc783baa) on the linux/amd64 offload
host. The report below is the script's. Before it: each run row's B/A
(Xo over Go) against release-vs-go-v0.38.1.md, a row worse by more than 3%
flagged and checked with a direct 0.38.1 against main `xo bench --ab`; then
bench/perf/micro13 built by 0.38.1, by main and by Go on the four platforms.

## Against 0.38.1

| row | 0.38.1 Xo/Go | 79bee8d83 Xo/Go | check: 0.38.1 against main, B/A |
|---|---|---|---|
| llvm fib | 0.942 | 1.351 flagged | darwin/arm64 0.990 (0.947 to 1.018), linux/arm64 1.002 (0.996 to 1.012): within noise, 9 rounds |
| llvm sieve | 0.946 | 0.708 | |
| llvm nbody | 1.071 | 1.054 | darwin/arm64 1.001 (0.996 to 1.008), within noise |
| llvm sort | 0.813 | 0.825 | |
| llvm interp | 0.319 | 0.298 | |
| llvm collatz | 0.903 | 0.971 flagged | darwin/arm64 1.004 (0.994 to 1.01), within noise |
| llvm strings | 0.485 | 0.497 | |
| coll putget | 0.600 | 0.627 | |
| coll get | 0.649 | 0.608 | |
| coll push | 0.752 | 0.689 | |
| coll update | 0.605 | 0.610 | |
| coll iter | 0.253 | 0.270 | |
| coll copymut | 0.681 | 0.680 | |
| coll branch | 0.026 | 0.025 | |
| conc spawn | 0.426 | 0.416 | |
| conc fanout | 0.635 | 0.580 | |
| conc pingpong | 0.925 | 0.921 | |
| conc mutex | 0.284 | 0.320 | |
| conc rwmutex | 0.928 | 1.001 flagged | darwin/arm64 1.022 (0.982 to 1.048), within noise, 9 rounds |
| conc pmap | 0.990 | 0.971 | |
| base64 test cold | 0.640 | 0.670 | |
| base64 test edit | 0.843 | 0.824 | |
| json test cold | 0.369 | 0.495 flagged | 0.38.1 against main (same cold caches, 5 rounds): 1.124 (1.088 to 1.132) worse; main's xo on the 0.38.1 std 1.092 (1.078 to 1.11) worse, so most of it is the compiler; 3c2a7722c's TokenReader tests 1.015 |
| json test edit | 0.246 | 0.303 flagged | 0.38.1 against main: 1.003 (0.983 to 1.031), within noise |

json test cold is the one confirmed slower row (still 0.495 of go test's time):
one XO_PROF of each compiler on the 0.38.1 std's cold json test puts
arm64.overlaps_sorted (regalloc.xo) at 40 against 130 ms and drop List[Int] at
40 against 170 ms; program builds (the compile table) are unchanged.

Cause and fix (Xo Lang 1, 2026-11-05): decision 0488's ShareIn (1f648265b)
asked made_here, which gave up after 1,000 steps, so the later pushes of a
long list literal (the json test's GO_CASES: 2,059 on linux) all got the
ShareIn check, which bloated register allocation (overlaps_sorted) and the
emit phase; fbd8b4d2e memoizes made_here with no limit. It landed after this
run (main 79bee8d83) and is in the 0.39.0 release (the respin). Cold json
test, `xo bench --ab`, linux/arm64 offload host, 0.38.1 std, 5 rounds: the
0.38.1 release against the 0.39.0 release archive, wall 1.01 (1.007 to
1.014), CPU 1.011 within noise; the 0.39.0 archive against an s2 of main
ddd2ed554 (0.39.0 std), wall 1.007 and 1.009, CPU 1.002 and 1.006, within
noise, twice.

HTTP (linux/arm64, 4 pads): main against 0.38.1 is 0.995 to 1.000 of its
req/s on every row (placement, by the pad sweep).

## micro13 on four platforms

bench/perf/micro13 at 79bee8d83 built with --release by the 0.38.1 release
(darwin-arm64 archive, cross), by main's xo, and by Go go1.26.1; best of 9
interleaved runs, milliseconds, 0.38.1 / main / Go. linux/arm64 and linux/amd64
on the offload hosts (idle), darwin/arm64 on the Mac, darwin/amd64 on the
owner's Intel laptop (load 2.5).

| workload | linux/arm64 | linux/amd64 | darwin/arm64 | darwin/amd64 |
|---|---|---|---|---|
| fib32 | 4 / 4 / 4 | 6 / 6 / 6 | 4 / 4 / 4 | 8 / 8 / 9 |
| json10kx20 | 50 / 61 / 160 | 84 / 47 / 161 | 53 / 52 / 147 | 159 / 110 / 330 |
| map1M | 50 / 51 / 59 | 75 / 68 / 89 | 45 / 43 / 63 | 159 / 149 / 167 |
| sort1M | 53 / 53 / 54 | 69 / 61 / 61 | 57 / 57 / 59 | 88 / 87 / 74 |
| spawn100k | 23 / 24 / 19 | 14 / 14 / 11 | 26 / 25 / 22 | 31 / 32 / 26 |
| strbuild1M | 5 / 5 / 16 | 8 / 6 / 24 | 5 / 5 / 14 | 19 / 17 / 47 |

linux/arm64 json10kx20's 50 against 61 is noise: an `xo bench --ab` of the
same two binaries, 9 rounds, reads 0.999 (0.991 to 1.014); spawn100k 0.955
(0.897 to 1.02, CPU 0.955 better). Main is behind Go on spawn100k everywhere
and on sort1M on darwin/amd64.

## The script's report


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

### Compile and size

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB | edit's path |
|---|---|---|---|---|---|---|---|---|
| llvm | go | 1540 | 40 | 40 | 100 | 294 / 26 / 67 | 2.39 | - |
| coll | go | 1520 | 40 | 40 | 110 | 290 / 25 / 68 | 2.43 | - |
| conc | go | 1480 | 40 | 30 | 100 | 316 / 26 / 68 | 2.40 | - |
| llvm-fib | xo | 1180 | 60 | 30 | 40 | 244 / 40 / 41 | 1.31 | separate, std records |
| llvm-sieve | xo | 1040 | 60 | 30 | 40 | 245 / 39 / 41 | 1.31 | separate, std records |
| llvm-nbody | xo | 1090 | 60 | 40 | 40 | 248 / 40 / 42 | 1.31 | separate, std records |
| llvm-sort | xo | 1080 | 60 | 40 | 40 | 247 / 40 / 41 | 1.31 | separate, std records |
| llvm-interp | xo | 1020 | 60 | 40 | 40 | 250 / 42 / 42 | 1.31 | separate, std records |
| llvm-collatz | xo | 1030 | 60 | 40 | 40 | 249 / 40 / 41 | 1.31 | separate, std records |
| llvm-strings | xo | 1060 | 60 | 30 | 30 | 246 / 39 / 41 | 1.31 | separate, std records |
| coll | xo | 1070 | 120 | 100 | 100 | 248 / 51 / 51 | 1.43 | separate, std records |
| conc-spawn | xo | 1230 | 60 | 30 | 40 | 245 / 40 / 41 | 1.39 | separate, std records |
| conc-fanout | xo | 1230 | 60 | 30 | 30 | 246 / 39 / 41 | 1.39 | separate, std records |
| conc-pingpong | xo | 1180 | 60 | 40 | 40 | 244 / 46 / 47 | 1.44 | separate, std records |
| conc-mutex | xo | 1210 | 60 | 40 | 40 | 241 / 40 / 41 | 1.45 | separate, std records |
| conc-rwmutex | xo | 1210 | 60 | 40 | 40 | 246 / 41 / 42 | 1.45 | separate, std records |
| conc-pmap | xo | 1220 | 60 | 40 | 40 | 245 / 39 / 42 | 1.41 | separate, std records |

The Go column builds one binary per group (llvm, coll, conc); the Xo
programs are one per workload except coll.

### Run (xo bench --ab, A = Go, B = Xo, 3 rounds)

#### llvm fib (load 5 at start)

```
A: llvm.go fib 
B: llvm-fib.xo fib 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.58/5.58 5.58/5.58 5.58/5.58

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               233.4       315.3       5.118% 4.648%     1.351   1.193 to 1.441    worse (n=3)
cpu_ms                232.7       302.4       4.969% 5.99%      1.299   1.172 to 1.446    worse (n=3)
max_rss_kb            4112        2592        1.362% 0.926%     0.63    0.63 to 0.635     better (n=3)
load: median 5.58 (A) / 5.58 (B); loaded: 0 of 3 rounds (at or above 8.58; baseline 5.58 + 3.0, at least 6.0)
```

#### llvm sieve (load 5 at start)

```
A: llvm.go sieve 
B: llvm-sieve.xo sieve 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.58/5.58 8.02/8.02 8.02/8.02

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               172.1       121.9       17.8% 16.46%      0.708   0.708 to 0.926    better (n=3)
cpu_ms                182.1       115.0       9.673% 11.4%      0.632   0.58 to 0.633     better (n=3)
max_rss_kb            46576       13232       1.546% 0.181%     0.284   0.283 to 0.293    better (n=3)
load: median 8.02 (A) / 8.02 (B); loaded: 0 of 3 rounds (at or above 8.58; baseline 5.58 + 3.0, at least 6.0)
```

#### llvm nbody (load 5 at start)

```
A: llvm.go nbody 
B: llvm-nbody.xo nbody 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.61/5.61 5.61/5.61 5.61/5.61

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               164.2       173.1       0.274% 0.214%     1.054   1.054 to 1.058    worse (n=3)
cpu_ms                163.3       172.1       0.205% 0.139%     1.054   1.054 to 1.057    worse (n=3)
max_rss_kb            4144        2592        0.193% 0%         0.625   0.625 to 0.628    better (n=3)
load: median 5.61 (A) / 5.61 (B); loaded: 0 of 3 rounds (at or above 8.61; baseline 5.61 + 3.0, at least 6.0)
```

#### llvm sort (load 5 at start)

```
A: llvm.go sort 
B: llvm-sort.xo sort 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.61/5.61 5.61/5.61 5.61/5.61

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               273.1       225.2       0.615% 0.508%     0.825   0.808 to 0.827    better (n=3)
cpu_ms                288.9       223.3       1.155% 0.605%     0.773   0.751 to 0.778    better (n=3)
max_rss_kb            76112       26736       26.42% 0.09%      0.351   0.259 to 0.425    better (n=3)
load: median 5.61 (A) / 5.61 (B); loaded: 0 of 3 rounds (at or above 8.61; baseline 5.61 + 3.0, at least 6.0)
```

#### llvm interp (load 5 at start)

```
A: llvm.go interp 
B: llvm-interp.xo interp 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.61/5.61 5.61/5.72 5.72/5.72

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               146.1       43.6        0.278% 11.51%     0.298   0.265 to 0.335    better (n=3)
cpu_ms                145.3       42.79       0.235% 11.58%     0.294   0.261 to 0.33     better (n=3)
max_rss_kb            4304        2592        0.743% 0.926%     0.602   0.6 to 0.613      better (n=3)
load: median 5.61 (A) / 5.72 (B); loaded: 0 of 3 rounds (at or above 8.61; baseline 5.61 + 3.0, at least 6.0)
```

#### llvm collatz (load 5 at start)

```
A: llvm.go collatz 
B: llvm-collatz.xo collatz 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.72/5.72 5.72/5.72 5.72/5.72

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               502.0       487.6       0.303% 0.179%     0.971   0.966 to 0.975    better (n=3)
cpu_ms                501.2       486.5       0.35% 0.288%      0.971   0.963 to 0.975    better (n=3)
max_rss_kb            4176        2592        1.149% 0%         0.621   0.616 to 0.63     better (n=3)
load: median 5.72 (A) / 5.72 (B); loaded: 0 of 3 rounds (at or above 8.72; baseline 5.72 + 3.0, at least 6.0)
```

#### llvm strings (load 5 at start)

```
A: llvm.go strings 
B: llvm-strings.xo strings 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.72/5.72 5.72/5.72 5.43/5.43

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               93.53       46.47       2.369% 3.558%     0.497   0.458 to 0.515    better (n=3)
cpu_ms                129.4       45.53       2.791% 3.437%     0.352   0.32 to 0.361     better (n=3)
max_rss_kb            26992       18288       9.455% 0.131%     0.678   0.657 to 0.803    better (n=3)
load: median 5.72 (A) / 5.72 (B); loaded: 0 of 3 rounds (at or above 8.72; baseline 5.72 + 3.0, at least 6.0)
```

#### coll putget (load 5 at start)

```
A: coll.go putget 
B: coll.xo putget 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.43/5.43 5.43/5.43 5.43/5.43

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               50.73       31.81       0.744% 2.025%     0.627   0.616 to 0.632    better (n=3)
cpu_ms                49.85       30.85       0.631% 2.515%     0.619   0.604 to 0.627    too coarse (n=3)
max_rss_kb            9088        5936        2.289% 0%         0.653   0.641 to 0.671    better (n=3)
load: median 5.43 (A) / 5.43 (B); loaded: 0 of 3 rounds (at or above 8.43; baseline 5.43 + 3.0, at least 6.0)
```

#### coll get (load 5 at start)

```
A: coll.go get 
B: coll.xo get 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.43/5.43 5.43/5.43 5.43/5.43

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               22.5        13.69       2.937% 0.77%      0.608   0.582 to 0.613    better (n=3)
cpu_ms                21.54       13.0        2.454% 0.969%     0.604   0.576 to 0.605    too coarse (n=3)
max_rss_kb            7120        5952        0.449% 0.403%     0.836   0.828 to 0.842    better (n=3)
load: median 5.43 (A) / 5.43 (B); loaded: 0 of 3 rounds (at or above 8.43; baseline 5.43 + 3.0, at least 6.0)
```

#### coll push (load 5 at start)

```
A: coll.go push 
B: coll.xo push 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.43/5.43 5.43/5.43 5.43/5.43

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               12.8        8.816       3.499% 1.872%     0.689   0.661 to 0.709    better (n=3)
cpu_ms                17.97       7.957       4.93% 2.935%      0.443   0.425 to 0.468    too coarse (n=3)
max_rss_kb            54320       26944       3.343% 0.089%     0.496   0.465 to 0.497    better (n=3)
load: median 5.43 (A) / 5.43 (B); loaded: 0 of 3 rounds (at or above 8.43; baseline 5.43 + 3.0, at least 6.0)
```

#### coll update (load 5 at start)

```
A: coll.go update 
B: coll.xo update 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.43/5.43 5.43/5.43 5.43/5.43

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               31.81       19.4        1.374% 0.585%     0.61    0.606 to 0.617    better (n=3)
cpu_ms                30.5        18.59       1.935% 0.261%     0.61    0.595 to 0.617    too coarse (n=3)
max_rss_kb            8816        5984        1.18% 0.401%      0.679   0.669 to 0.685    better (n=3)
load: median 5.43 (A) / 5.43 (B); loaded: 0 of 3 rounds (at or above 8.43; baseline 5.43 + 3.0, at least 6.0)
```

#### coll iter (load 5 at start)

```
A: coll.go iter 
B: coll.xo iter 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.43/5.43 5.43/5.43 5.43/5.43

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               23.13       6.233       2.421% 6.313%     0.27    0.254 to 0.276    better (n=3)
cpu_ms                22.43       5.337       2.88% 7.401%      0.238   0.23 to 0.251     too coarse (n=3)
max_rss_kb            7120        5936        0.337% 0.135%     0.834   0.832 to 0.84     better (n=3)
load: median 5.43 (A) / 5.43 (B); loaded: 0 of 3 rounds (at or above 8.43; baseline 5.43 + 3.0, at least 6.0)
```

#### coll copymut (load 5 at start)

```
A: coll.go copymut 
B: coll.xo copymut 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.43/5.43 5.43/5.43 5.43/5.43

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               14.31       9.729       1.16% 1.1%        0.68    0.668 to 0.693    better (n=3)
cpu_ms                14.58       9.003       1.612% 1.277%     0.617   0.604 to 0.635    too coarse (n=3)
max_rss_kb            10400       5936        1.077% 0%         0.571   0.567 to 0.58     better (n=3)
load: median 5.43 (A) / 5.43 (B); loaded: 0 of 3 rounds (at or above 8.43; baseline 5.43 + 3.0, at least 6.0)
```

#### coll branch (load 5 at start)

```
A: coll.go branch 
B: coll.xo branch 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.43/5.43 5.43/5.43 5.43/5.43

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               210.3       5.157       1.984% 1.338%     0.025   0.024 to 0.025    better (n=3)
cpu_ms                342.2       4.353       3.97% 1.057%      0.013   0.013 to 0.014    better (n=3)
max_rss_kb            14784       5936        4.6% 0%           0.402   0.375 to 0.41     better (n=3)
load: median 5.43 (A) / 5.43 (B); loaded: 0 of 3 rounds (at or above 8.43; baseline 5.43 + 3.0, at least 6.0)
```

#### conc spawn (load 5 at start)

```
A: conc.go spawn 
B: conc-spawn.xo spawn 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.43/5.43 5.43/5.43 5.43/5.43

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               53.71       22.35       4.624% 1.107%     0.416   0.398 to 0.431    better (n=3)
cpu_ms                181.5       26.65       4.489% 9.791%     0.147   0.13 to 0.147     better (n=3)
max_rss_kb            11712       5888        4.577% 6.114%     0.503   0.474 to 0.503    better (n=3)
load: median 5.43 (A) / 5.43 (B); loaded: 0 of 3 rounds (at or above 8.43; baseline 5.43 + 3.0, at least 6.0)
```

#### conc fanout (load 5 at start)

```
A: conc.go fanout 
B: conc-fanout.xo fanout 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.43/5.43 5.43/5.43 5.43/5.43

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               27.51       15.95       3.47% 1.988%      0.58    0.563 to 0.58     better (n=3)
cpu_ms                78.24       15.96       4.101% 4.378%     0.204   0.19 to 0.218     too coarse (n=3)
max_rss_kb            11200       4752        0.929% 16.84%     0.424   0.367 to 0.507    better (n=3)
load: median 5.43 (A) / 5.43 (B); loaded: 0 of 3 rounds (at or above 8.43; baseline 5.43 + 3.0, at least 6.0)
```

#### conc pingpong (load 5 at start)

```
A: conc.go pingpong 
B: conc-pingpong.xo pingpong 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.43/5.43 5.43/5.43 5.43/5.43

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               19.7        18.13       4.656% 4.638%     0.921   0.914 to 0.947    better (n=3)
cpu_ms                26.75       17.3        1.544% 1.474%     0.647   0.641 to 0.653    too coarse (n=3)
max_rss_kb            4128        2736        2.326% 0%         0.663   0.638 to 0.668    better (n=3)
load: median 5.43 (A) / 5.43 (B); loaded: 0 of 3 rounds (at or above 8.43; baseline 5.43 + 3.0, at least 6.0)
```

#### conc mutex (load 5 at start)

```
A: conc.go mutex 
B: conc-mutex.xo mutex 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.43/5.43 5.43/5.43 5.43/5.43

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               23.56       7.534       2.417% 11.39%     0.32    0.287 to 0.368    better (n=3)
cpu_ms                95.5        6.55        8.99% 5.924%      0.069   0.058 to 0.075    too coarse (n=3)
max_rss_kb            4688        2704        1.024% 0.888%     0.577   0.567 to 0.589    better (n=3)
load: median 5.43 (A) / 5.43 (B); loaded: 0 of 3 rounds (at or above 8.43; baseline 5.43 + 3.0, at least 6.0)
```

#### conc rwmutex (load 5 at start)

```
A: conc.go rwmutex 
B: conc-rwmutex.xo rwmutex 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.43/5.43 5.43/5.43 5.43/5.43

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               9.044       9.051       3.969% 1.591%     1.001   0.943 to 1.033    within noise (n=3)
cpu_ms                12.98       8.249       5.149% 1.576%     0.635   0.582 to 0.655    too coarse (n=3)
max_rss_kb            4768        2720        1.174% 0.882%     0.57    0.569 to 0.579    better (n=3)
load: median 5.43 (A) / 5.43 (B); loaded: 0 of 3 rounds (at or above 8.43; baseline 5.43 + 3.0, at least 6.0)
```

#### conc pmap (load 5 at start)

```
A: conc.go pmap 
B: conc-pmap.xo pmap 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.43/5.43 5.43/5.43 6.11/6.11

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               62.56       60.74       3.267% 0.277%     0.971   0.93 to 0.995     better (n=3)
cpu_ms                560.6       557.1       0.477% 0.379%     0.994   0.991 to 0.994    better (n=3)
max_rss_kb            6784        5424        0.708% 0.59%      0.8     0.792 to 0.811    better (n=3)
load: median 5.43 (A) / 5.43 (B); loaded: 0 of 3 rounds (at or above 8.43; baseline 5.43 + 3.0, at least 6.0)
```


### Test (xo bench --ab, A = go test, B = xo test --std-module, 3 rounds)

Cold empties every cache before each round (Go with -count=1); edit appends one
function to the module under test before each round (Go -count=1). Copies of GOROOT
and the release's std.

#### base64 (encoding/base64) cold (load 5 at start)

```
A: goroot/bin/go test -count=1 encoding/base64 
B: xo test --std-module=native/base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.34/5.34 6.12/6.12 5.79/5.79

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               2229        1494        0.645% 1.162%     0.67    0.649 to 0.672    better (n=3)
cpu_ms                11515       2251        0.265% 2.263%     0.195   0.189 to 0.198    better (n=3)
max_rss_kb            307488      351488      2.443% 0.157%     1.143   1.115 to 1.174    worse (n=3)
load: median 5.79 (A) / 5.79 (B); loaded: 0 of 3 rounds (at or above 8.2; baseline 5.2 + 3.0, at least 6.0)
```

#### base64 (encoding/base64) edit (load 5 at start)

```
A: goroot/bin/go test -count=1 encoding/base64 
B: xo test --std-module=native/base64 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.44/5.44 5.44/5.44 5.44/5.44

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               479.3       394.7       1.409% 0.719%     0.824   0.802 to 0.834    better (n=3)
cpu_ms                966.6       400.0       1.784% 2.91%      0.414   0.406 to 0.441    better (n=3)
max_rss_kb            105088      208288      0.221% 0.165%     1.982   1.975 to 1.984    worse (n=3)
load: median 5.44 (A) / 5.44 (B); loaded: 0 of 3 rounds (at or above 8.44; baseline 5.44 + 3.0, at least 6.0)
```

#### json (encoding/json) cold (load 5 at start)

```
A: goroot/bin/go test -count=1 encoding/json 
B: xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.32/5.32 7.77/10.59 10.79/10.79

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               4945        2450        7.542% 13.52%     0.495   0.392 to 0.495    better (n=3)
cpu_ms                25252       3836        1.452% 13.15%     0.152   0.125 to 0.162    better (n=3)
max_rss_kb            310320      370144      4.434% 0.061%     1.193   1.117 to 1.22     worse (n=3)
load: median 7.77 (A) / 10.59 (B); loaded: 2 of 3 rounds (at or above 8.44; baseline 5.44 + 3.0, at least 6.0)
warning: the machine was busy through the run (median load at or above 8.44): wall and cpu ratios are not reliable; measure with HEAVY_CLASS=measure (agent-tools.md Measuring) or --max-load
```

#### json (encoding/json) edit (load 5 at start)

```
A: goroot/bin/go test -count=1 encoding/json 
B: xo test --std-module=native/json 
3 of 3 rounds used (0 loaded), loads A/B per round: 5.56/5.56 5.6/5.6 5.6/5.6

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               956.5       289.5       0.928% 0.298%     0.303   0.301 to 0.306    better (n=3)
cpu_ms                2465        361.7       3.736% 0.242%     0.147   0.146 to 0.157    better (n=3)
max_rss_kb            162064      223568      0.148% 0.701%     1.38    1.368 to 1.39     worse (n=3)
load: median 5.6 (A) / 5.6 (B); loaded: 0 of 3 rounds (at or above 8.56; baseline 5.56 + 3.0, at least 6.0)
```


### HTTP server (bench/perf/jitter, linux/arm64 offload host)

- host load 0.15 2.55 3.27 1/310 2772276
- release-vs-go-http rvg89428: 79bee8d8302 on Linux aarch64, 12 CPUs, go1.26.1
- xo xo 0.38.1 linux/arm64; base xo 0.38.1 linux/arm64
- loadgen flags: -c C -rate R -d 10s -warmup 2s -awake (default on in open loop); servers on CPUs 0-5, loadgen on 6-11
- Go's /alloc rows are its /alloc-hand (json.Marshal written by hand, as Xo's derive encodes).
- Built at each XO_ARM64_PAD of HTTP_PADS: the table pools every pad's rounds; the pad sweep verdict below judges xo against base.
- Rounds: 12 per row; each cell is the median, its range in brackets; CPU is the server's CPU microseconds per request.

| row | server | req/s | p50 us | p99 us | p99.9 us | max us | CPU us/req | errors |
|---|---|---|---|---|---|---|---|---|
| /alloc c32 | xo | 168,663 [166,952 to 171,245] | 173 [171 to 176] | 458 [450 to 465] | 1,304 [1,215 to 1,410] | 6,306 [6,117 to 10,498] | 34.4 [33.6 to 34.6] | 0 [0 to 0] |
| /alloc c32 | base | 168,587 [166,495 to 172,516] | 173 [170 to 175] | 457 [447 to 468] | 1,244 [1,168 to 1,524] | 6,227 [5,774 to 18,113] | 34.1 [33.2 to 34.8] | 0 [0 to 0] |
| /alloc c32 | go | 28,095 [27,929 to 28,327] | 482 [482 to 487] | 5,487 [5,434 to 5,589] | 8,054 [8,022 to 8,139] | 16,571 [14,119 to 16,990] | 117.4 [116.4 to 119.1] | 0 [0 to 0] |
| /alloc c64 open 10000/s | xo | 10,000 [10,000 to 10,000] | 79 [78 to 79] | 114 [108 to 126] | 144 [133 to 167] | 206 [190 to 2,222] | 52.0 [51.1 to 52.8] | 0 [0 to 0] |
| /alloc c64 open 10000/s | base | 10,000 [10,000 to 10,000] | 78 [78 to 80] | 114 [109 to 125] | 144 [134 to 162] | 276 [186 to 1,176] | 51.8 [51.3 to 53.1] | 0 [0 to 0] |
| /alloc c64 open 10000/s | go | 10,000 [10,000 to 10,000] | 102 [98 to 102] | 392 [357 to 397] | 505 [462 to 524] | 1,316 [1,003 to 1,780] | 122.6 [115.5 to 124.7] | 0 [0 to 0] |
| /alloc c64 open 20000/s | xo | 20,000 [20,000 to 20,000] | 92 [91 to 92] | 144 [142 to 145] | 168 [165 to 169] | 267 [220 to 882] | 57.6 [56.8 to 58.1] | 0 [0 to 0] |
| /alloc c64 open 20000/s | base | 20,000 [20,000 to 20,000] | 91 [91 to 92] | 144 [142 to 155] | 168 [166 to 283] | 266 [215 to 2,624] | 57.4 [56.9 to 58.1] | 0 [0 to 0] |
| /alloc c64 open 20000/s | go | 20,000 [20,000 to 20,000] | 184 [146 to 189] | 650 [548 to 652] | 884 [769 to 923] | 2,157 [1,757 to 4,277] | 126.5 [113.9 to 127.9] | 0 [0 to 0] |
| /alloc c256 | xo | 179,353 [176,418 to 183,095] | 1,240 [1,213 to 1,268] | 5,152 [5,133 to 5,250] | 6,466 [6,377 to 6,884] | 10,292 [9,582 to 13,273] | 33.4 [32.7 to 34.0] | 0 [0 to 0] |
| /alloc c256 | base | 179,956 [178,270 to 185,083] | 1,238 [1,200 to 1,248] | 5,173 [5,079 to 5,241] | 6,478 [6,308 to 6,643] | 10,460 [9,637 to 16,999] | 33.3 [32.3 to 33.6] | 0 [0 to 0] |
| /alloc c256 | go | 57,447 [57,029 to 57,481] | 381 [380 to 389] | 38,134 [38,042 to 38,418] | 57,159 [56,932 to 57,931] | 117,626 [117,018 to 127,243] | 81.4 [80.9 to 82.0] | 0 [0 to 0] |
| /hello c32 | xo | 355,324 [353,451 to 357,696] | 86 [86 to 87] | 220 [219 to 225] | 318 [313 to 340] | 4,224 [3,327 to 10,209] | 7.3 [7.3 to 7.4] | 0 [0 to 0] |
| /hello c32 | base | 356,637 [349,745 to 359,068] | 86 [85 to 88] | 220 [219 to 227] | 321 [317 to 351] | 3,808 [3,506 to 10,137] | 7.3 [7.2 to 7.4] | 0 [0 to 0] |
| /hello c32 | go | 277,209 [275,911 to 277,647] | 102 [101 to 103] | 632 [625 to 632] | 840 [831 to 842] | 3,439 [3,256 to 10,140] | 12.3 [12.3 to 12.3] | 0 [0 to 0] |
| /hello c256 | xo | 886,702 [878,998 to 895,692] | 258 [256 to 261] | 822 [800 to 833] | 1,666 [1,603 to 1,814] | 8,012 [6,692 to 9,845] | 5.9 [5.9 to 6.0] | 0 [0 to 0] |
| /hello c256 | base | 890,511 [878,468 to 896,000] | 257 [256 to 260] | 827 [803 to 845] | 1,688 [1,618 to 1,775] | 8,324 [6,678 to 10,548] | 5.9 [5.9 to 5.9] | 0 [0 to 0] |
| /hello c256 | go | 488,410 [487,102 to 490,340] | 308 [305 to 310] | 2,608 [2,600 to 2,636] | 4,006 [3,965 to 4,337] | 8,721 [8,042 to 10,249] | 10.0 [9.9 to 10.0] | 0 [0 to 0] |

- /alloc c32, xo/go medians: req/s 6.003, p50 us 0.359, p99 us 0.083, p99.9 us 0.162, max us 0.381, CPU us/req 0.293
- /alloc c32, xo/base medians: req/s 1.000, p50 us 1.000, p99 us 1.002, p99.9 us 1.048, max us 1.013, CPU us/req 1.008
- /alloc c64 open 10000/s, xo/go medians: req/s 1.000, p50 us 0.775, p99 us 0.292, p99.9 us 0.284, max us 0.156, CPU us/req 0.425
- /alloc c64 open 10000/s, xo/base medians: req/s 1.000, p50 us 1.013, p99 us 1.004, p99.9 us 0.993, max us 0.743, CPU us/req 1.005
- /alloc c64 open 20000/s, xo/go medians: req/s 1.000, p50 us 0.497, p99 us 0.222, p99.9 us 0.190, max us 0.124, CPU us/req 0.456
- /alloc c64 open 20000/s, xo/base medians: req/s 1.000, p50 us 1.005, p99 us 1.000, p99.9 us 1.003, max us 1.004, CPU us/req 1.004
- /alloc c256, xo/go medians: req/s 3.122, p50 us 3.256, p99 us 0.135, p99.9 us 0.113, max us 0.087, CPU us/req 0.411
- /alloc c256, xo/base medians: req/s 0.997, p50 us 1.002, p99 us 0.996, p99.9 us 0.998, max us 0.984, CPU us/req 1.004
- /hello c32, xo/go medians: req/s 1.282, p50 us 0.843, p99 us 0.349, p99.9 us 0.379, max us 1.228, CPU us/req 0.598
- /hello c32, xo/base medians: req/s 0.996, p50 us 1.000, p99 us 1.002, p99.9 us 0.992, max us 1.109, CPU us/req 1.010
- /hello c256, xo/go medians: req/s 1.815, p50 us 0.838, p99 us 0.315, p99.9 us 0.416, max us 0.919, CPU us/req 0.593
- /hello c256, xo/base medians: req/s 0.996, p50 us 1.004, p99 us 0.994, p99.9 us 0.988, max us 0.963, CPU us/req 1.009

pad sweep verdict (reference base): each cell is the median over pads of each pad's median over rounds, the pad medians' range in brackets; slower or faster only when the ranges do not overlap

| row | group | pads | req/s | CPU us/req | req/s vs ref | verdict |
|---|---|---|---|---|---|---|
| /alloc c32 | base | 4 | 168,658 [167,426 to 172,382] | 34.16 [33.27 to 34.44] |  |  |
| /alloc c32 | go | 1 | 28,095 [28,095 to 28,095] | 117.42 [117.42 to 117.42] | 0.167 | one pad: not judged |
| /alloc c32 | xo | 4 | 168,663 [167,089 to 170,237] | 34.40 [33.71 to 34.47] | 1.000 | placement |
| /alloc c64 open 10000/s | base | 4 | 10,000 [10,000 to 10,000] | 52.10 [51.60 to 52.80] |  |  |
| /alloc c64 open 10000/s | go | 1 | 10,000 [10,000 to 10,000] | 122.60 [122.60 to 122.60] | 1.000 | one pad: not judged |
| /alloc c64 open 10000/s | xo | 4 | 10,000 [10,000 to 10,000] | 52.00 [51.60 to 52.60] | 1.000 | placement |
| /alloc c64 open 20000/s | base | 4 | 20,000 [20,000 to 20,000] | 57.40 [57.05 to 57.80] |  |  |
| /alloc c64 open 20000/s | go | 1 | 20,000 [20,000 to 20,000] | 126.50 [126.50 to 126.50] | 1.000 | one pad: not judged |
| /alloc c64 open 20000/s | xo | 4 | 20,000 [20,000 to 20,000] | 57.50 [56.85 to 58.10] | 1.000 | placement |
| /alloc c256 | base | 4 | 180,094 [178,718 to 183,864] | 33.26 [32.59 to 33.51] |  |  |
| /alloc c256 | go | 1 | 57,447 [57,447 to 57,447] | 81.38 [81.38 to 81.38] | 0.319 | one pad: not judged |
| /alloc c256 | xo | 4 | 179,117 [177,344 to 182,244] | 33.45 [32.86 to 33.78] | 0.995 | placement |
| /hello c32 | base | 4 | 356,637 [351,008 to 357,449] | 7.28 [7.24 to 7.32] |  |  |
| /hello c32 | go | 1 | 277,209 [277,209 to 277,209] | 12.29 [12.29 to 12.29] | 0.777 | one pad: not judged |
| /hello c32 | xo | 4 | 355,155 [353,973 to 356,339] | 7.34 [7.34 to 7.38] | 0.996 | placement |
| /hello c256 | base | 4 | 890,834 [888,324 to 892,394] | 5.88 [5.87 to 5.89] |  |  |
| /hello c256 | go | 1 | 488,410 [488,410 to 488,410] | 10.01 [10.01 to 10.01] | 0.548 | one pad: not judged |
| /hello c256 | xo | 4 | 886,460 [885,338 to 890,690] | 5.93 [5.91 to 5.95] | 0.995 | placement |

### linux/amd64 micro set (bench/perf/micro13, the linux/amd64 offload host)

Note (2026-11-08): release-vs-go-amd64.sh ran these binaries from /mnt/c,
where drvfs adds about 6 ms of exec to every run (fib32 13.8 against 7.9 ms
from ext4), more for Go's larger binary, so the short rows' ratios against Go
read too favorable to Xo and small changes against the release before are
hidden. gotchas/wsl-drvfs-exec-cost.md; fixed in f26b951fb. The "micro13 on
four platforms" section above ran from ext4 and is not affected.

- base: xo 0.38.1 darwin/arm64; new: xo 0.38.1 darwin/arm64; Go go1.26.1; all built for linux/amd64 (xo with --release, as the owner's tests/13-perf)
- each block is xo bench --ab, A = base or go, B = new; ratio is new over A; a row more than 3% slower than base is FLAGGED

```
host Linux x86_64, 28 CPUs, load 0.00
#### fib32: base against new
A: rvg-micro13.base fib32 
B: rvg-micro13.new fib32 
5 of 5 rounds used (0 loaded), loads A/B per round: 0/0 0/0 0/0 0/0 0/0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               13.63       13.79       2.759% 1.766%     1.011   0.965 to 1.059    within noise (n=5)
cpu_ms                8.622       8.824       4.825% 1.53%      1.023   0.957 to 1.039    too coarse (n=5)
max_rss_kb            1144        1144        0% 0.35%          1.0     1.0 to 1.003      within noise (n=5)
load: median 0 (A) / 0 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0 + 3.0, at least 6.0)
#### fib32: go against new
A: rvg-micro13.go fib32 
B: rvg-micro13.new fib32 
5 of 5 rounds used (0 loaded), loads A/B per round: 0/0 0/0 0/0 0/0 0/0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               10.83       13.45       107.1% 2.613%     1.241   0.552 to 1.446    within noise (n=5)
cpu_ms                9.816       8.489       37.95% 3.793%     0.865   0.611 to 0.954    too coarse (n=5)
max_rss_kb            2608        1112        8.436% 0.36%      0.426   0.426 to 0.466    better (n=5)
load: median 0 (A) / 0 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0 + 3.0, at least 6.0)
#### map1M: base against new
A: rvg-micro13.base map1M 
B: rvg-micro13.new map1M 
5 of 5 rounds used (0 loaded), loads A/B per round: 0/0 0/0 0/0 0/0 0/0

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               92.78       87.8        13.67% 15.21%     0.946   0.869 to 1.013    within noise (n=5)
cpu_ms                83.68       82.71       13.86% 12.03%     0.988   0.887 to 1.016    too coarse (n=5)
max_rss_kb            52156       73472       0.429% 0%         1.409   1.403 to 1.409    worse (n=5)
load: median 0 (A) / 0 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0 + 3.0, at least 6.0)
#### map1M: go against new
A: rvg-micro13.go map1M 
B: rvg-micro13.new map1M 
5 of 5 rounds used (0 loaded), loads A/B per round: 0/0 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               112.4       84.74       3.194% 3.812%     0.754   0.71 to 0.825     better (n=5)
cpu_ms                111.4       77.18       6.086% 1.984%     0.693   0.656 to 0.768    better (n=5)
max_rss_kb            51220       73472       0.437% 0%         1.434   1.422 to 1.473    worse (n=5)
load: median 0.08 (A) / 0.08 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0 + 3.0, at least 6.0)
#### strbuild1M: base against new
A: rvg-micro13.base strbuild1M 
B: rvg-micro13.new strbuild1M 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               17.32       17.78       2.499% 4.216%     1.027   0.932 to 1.074    within noise (n=5)
cpu_ms                10.55       10.45       0.502% 9.517%     0.99    0.887 to 1.12     too coarse (n=5)
max_rss_kb            12320       12320       1.818% 0%         1.0     1.0 to 1.019      within noise (n=5)
load: median 0.08 (A) / 0.08 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.08 + 3.0, at least 6.0)
#### strbuild1M: go against new
A: rvg-micro13.go strbuild1M 
B: rvg-micro13.new strbuild1M 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               34.78       17.5        38.74% 2.409%     0.503   0.365 to 0.576    better (n=5)
cpu_ms                44.77       9.96        13.03% 5.663%     0.222   0.211 to 0.254    too coarse (n=5)
max_rss_kb            30772       12320       12.4% 0%          0.4     0.36 to 0.456     better (n=5)
load: median 0.08 (A) / 0.08 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.08 + 3.0, at least 6.0)
#### sort1M: base against new
A: rvg-micro13.base sort1M 
B: rvg-micro13.new sort1M 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               78.48       78.48       1.059% 0.973%     1.0     0.975 to 1.013    within noise (n=5)
cpu_ms                70.16       71.28       1.408% 0.518%     1.016   0.991 to 1.052    too coarse (n=5)
max_rss_kb            18324       18548       1.222% 0%         1.012   1.0 to 1.024      within noise (n=5)
load: median 0.08 (A) / 0.08 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.08 + 3.0, at least 6.0)
#### sort1M: go against new
A: rvg-micro13.go sort1M 
B: rvg-micro13.new sort1M 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               81.27       79.02       2.101% 0.691%     0.972   0.965 to 1.178    within noise (n=5)
cpu_ms                72.63       71.14       2.337% 1.681%     0.98    0.97 to 1.06      too coarse (n=5)
max_rss_kb            10676       18548       0.037% 0%         1.737   1.737 to 1.775    worse (n=5)
load: median 0.08 (A) / 0.08 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.08 + 3.0, at least 6.0)
#### json10kx20: base against new
A: rvg-micro13.base json10kx20 
B: rvg-micro13.new json10kx20 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08 0.08/0.08

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               94.65       79.62       3.03% 10.72%      0.841   0.784 to 1.012    within noise (n=5)
cpu_ms                87.56       72.31       4.987% 11.9%      0.826   0.754 to 1.011    too coarse (n=5)
max_rss_kb            13904       43092       1.611% 0.195%     3.099   3.099 to 3.156    worse (n=5)
load: median 0.08 (A) / 0.08 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.08 + 3.0, at least 6.0)
#### json10kx20: go against new
A: rvg-micro13.go json10kx20 
B: rvg-micro13.new json10kx20 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.08/0.08 0.08/0.15 0.15/0.15 0.15/0.15 0.15/0.15

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               192.0       77.17       0.616% 1.436%     0.402   0.392 to 0.407    better (n=5)
cpu_ms                236.1       70.34       3.183% 2.383%     0.298   0.293 to 0.313    better (n=5)
max_rss_kb            14132       43176       3.142% 0.195%     3.055   2.978 to 3.097    worse (n=5)
load: median 0.15 (A) / 0.15 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.08 + 3.0, at least 6.0)
#### spawn100k: base against new
A: rvg-micro13.base spawn100k 
B: rvg-micro13.new spawn100k 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.15/0.15 0.15/0.15 0.15/0.15 0.15/0.15 0.15/0.15

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               17.79       17.05       2.084% 6.239%     0.958   0.918 to 1.075    within noise (n=5)
cpu_ms                58.59       63.14       7.454% 7.937%     1.078   0.982 to 1.163    too coarse (n=5)
max_rss_kb            5292        5320        8.919% 6.09%      1.005   0.72 to 1.121     within noise (n=5)
load: median 0.15 (A) / 0.15 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.15 + 3.0, at least 6.0)
#### spawn100k: go against new
A: rvg-micro13.go spawn100k 
B: rvg-micro13.new spawn100k 
5 of 5 rounds used (0 loaded), loads A/B per round: 0.15/0.15 0.15/0.15 0.15/0.15 0.15/0.15 0.15/0.15

metric                A median    B median    spread A/B        B / A   95% interval      verdict
wall_ms               16.57       17.44       1.613% 2.97%      1.053   1.02 to 1.079     worse (n=5)
cpu_ms                37.52       61.9        7.383% 4.173%     1.65    1.571 to 1.84     too coarse (n=5)
max_rss_kb            8212        5452        8.134% 11.52%     0.664   0.646 to 0.932    better (n=5)
load: median 0.15 (A) / 0.15 (B); loaded: 0 of 5 rounds (at or above 6.0; baseline 0.15 + 3.0, at least 6.0)
OFFLOAD-RC=0
```
