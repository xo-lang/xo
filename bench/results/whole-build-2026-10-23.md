# Whole-build cache, stage 1 (decision 0360): 2026-10-23

`xo build` and `xo run` through `xo test`'s run cache (testcache.xo)
under XO_WHOLE_BUILD=on, with the build's output kept and printed again
on a hit. xo built from this change (wt-0360 on 673edcf6b), Apple M4 Pro,
darwin/arm64, gate class, other agents running (load 18 to 19 during the
`--ab` rounds, which alternate A and B).

## xo bench --ab, warm builds (10 rounds; A = XO_WHOLE_BUILD=off, B = on)

Each side warmed by three builds into its own output in the same
directory and cache. The CRUD server is bench/perf/http/xo; hello is a
one-function program with one test.

CRUD server:

    metric                A median    B median    spread A/B        B / A   95% interval      verdict
    wall_ms               559.8       55.67       1.753% 13.56%     0.099   0.091 to 0.106    better (n=10)
    cpu_ms                1740        40.0        7.04% 18.75%      0.023   0.017 to 0.024    better (n=10)
    max_rss_kb            546240      17168       0.484% 0.839%     0.031   0.031 to 0.032    better (n=10)

hello:

    metric                A median    B median    spread A/B        B / A   95% interval      verdict
    wall_ms               301.2       43.5        1.839% 5.034%     0.144   0.142 to 0.158    better (n=10)
    cpu_ms                770.0       30.0        3.896% 0%         0.039   0.039 to 0.041    better (n=10)
    max_rss_kb            308808      14728       0.776% 0.19%      0.048   0.047 to 0.048    better (n=10)

`--phase-times` puts the lookup at 28 to 35 ms of a hit (hashing every
std file, each module directory's files and listings, the module files
up from the working directory); the rest is xo's start and the copy.

## Smoke run (the CRUD server, then a small program)

- `xo build -o prog .`: 3.70 s cold, then 0.03 s on each of three hits,
  each printing the XO0540 warning and the `darwin/arm64: PATH` line as
  the build did.
- after an edit: a miss (0.64 s, phase check), then a hit (0.04 s).
- `xo test .` on the CRUD server (it has the warning): 1.71 s, then
  0.17 s on each hit with the warning printed. Before this change the
  run cache never stored a build that printed a warning: six warm `xo
  test` runs were 0.77 to 0.79 s, all misses.
- `xo run .` of a small program: 1.21 s, then 0.23 and 0.17 s.

TestWholeBuildCache covers the cases: unchanged and touched hit; an
edit, a new file, an XO_ variable, a flag, and a std file edited at the
same size with its modification time put back miss; two builds of one
output at once (missing, then hitting) leave a binary that runs; `xo
run` and `xo test` print the warning on a hit.

## Stage 2: the recorded inputs (later the same day)

The entry lists what the build's recording Fs and Env saw (decision 0360
stage 2) instead of the whole std tree. The same `--ab` script, the same
machine (load lower than above):

CRUD server:

    wall_ms               540.4       39.47       0.495% 4.519%     0.073   0.07 to 0.075     better (n=10)
    cpu_ms                1795        30.0        5.71% 33.33%      0.017   0.011 to 0.017    better (n=10)
    max_rss_kb            546896      14736       0.378% 0.109%     0.027   0.027 to 0.027    better (n=10)

hello:

    wall_ms               277.0       40.27       5.571% 21.43%     0.145   0.13 to 0.164     better (n=10)
    cpu_ms                740.0       25.0        5.405% 40.0%      0.034   0.027 to 0.041    better (n=10)
    max_rss_kb            309904      14736       1.402% 0.19%      0.048   0.047 to 0.048    better (n=10)

A hello build's entry lists 445 std files read (the checker loads most of
std), a walk of std, std/rt's listing, the module directory, 15 probes
(the xo.work search up to /, vendor/modules.txt, xo.mod) and the XO_
variables the build read, so the lookup still hashes most of std.
