# Separately compiled edit builds on Linux (decision 0388): fib and the HTTP service, separate vs whole vs Go

2026-10-28, linux/arm64 in Docker on the development Mac (debian trixie-slim, 12 CPUs, scripts/docker-linux.sh), measure class (quiet machine). xo of main at 7193d5318 (separate compilation on by default on linux since 84939ecb6), cross-built for linux/arm64 with `--backend=arm64`; Go 1.26.1 linux/arm64 (go.dev's tarball, sha256 checked). std's variants built first with `xo dev stdcache --conc=0` and `--conc=1`, so the default found them. Each build warmed three times, then 20 interleaved edits per program: fib, the fib argument in main.xo (main.go's constant for Go); HTTP, an error message's text in bench/perf/http/xo/api.xo (handlers.go's for Go). Separate is the default build, whole is `XO_SEPCOMP=0`, Go is `go build` with a warm GOCACHE. Wall time is `date +%s%N` around the build. 120 of 120 builds succeeded; the three fib binaries print the same line.

| program | separate ms (median / mean) | whole ms | Go ms |
|---|---|---|---|
| fib | 196.5 / 198.7 | 350.0 / 349.6 | 161.5 / 166.7 |
| HTTP | 453.5 / 476.9 | 804.0 / 872.5 | 390.5 / 402.7 |

(min to max: fib separate 189 to 224, whole 319 to 383, Go 152 to 224; HTTP separate 423 to 600, whole 779 to 1717, Go 382 to 483.)

Phase means (ms, --phase-times):

| phase | fib separate | fib whole | HTTP separate | HTTP whole |
|---|---|---|---|---|
| check | 67.8 | 65.9 | 122.9 | 119.5 |
| ssa | 20.6 | 48.1 | 51.0 | 110.2 |
| insert_rc | 3.8 | 34.6 | 26.4 | 91.5 |
| inline | 3.0 | 33.8 | 30.4 | 107.0 |
| emit | 4.7 | 14.6 | 25.1 | 41.0 |
| assemble | 2.4 | 7.0 | 3.0 | 18.4 |
| link | 51.7 | 20.1 | 103.2 | 69.5 |

Separate compilation takes an HTTP edit from 804 to 454 ms (median) and fib from 350 to 197 ms, within 1.17x and 1.22x of Go's. The link is the outlier: a separate build's link is 2.6x the whole program's for fib and 1.5x for HTTP. On darwin it is about even (sepcomp-edit-http-2026-10-28.md: 43.5 separate against 41.2 whole). The Linux store link is the next target (decision 0388).

The figures are Docker on the Mac (a Linux VM, the sources on a bind mount), so the absolute times are not a Linux machine's: check alone is about twice darwin's native 56 ms. The ratios between the three columns, measured interleaved in one container, are what this records.

Script: ~/xo-lead/notes/xo-lang-2/linedit.sh and linedit-inner.sh (Xo Lang 2's notes).

Raw (program, mode, wall ms, exit, phases):

```
fib sep 202 rc=0 check=68 ssa=24 insert_rc=2 inline=2 emit=4 assemble=2 link=51 
fib whole 369 rc=0 check=66 ssa=48 insert_rc=37 inline=37 emit=17 assemble=7 link=20 
fib go 168 rc=0 
fib sep 201 rc=0 check=66 ssa=24 insert_rc=2 inline=2 emit=5 assemble=2 link=50 
fib whole 365 rc=0 check=64 ssa=48 insert_rc=36 inline=39 emit=18 assemble=7 link=20 
fib go 172 rc=0 
fib sep 204 rc=0 check=65 ssa=23 insert_rc=5 inline=2 emit=5 assemble=2 link=51 
fib whole 357 rc=0 check=65 ssa=48 insert_rc=35 inline=35 emit=16 assemble=6 link=21 
fib go 179 rc=0 
fib sep 198 rc=0 check=66 ssa=23 insert_rc=6 inline=3 emit=4 assemble=2 link=51 
fib whole 383 rc=0 check=64 ssa=56 insert_rc=35 inline=35 emit=17 assemble=7 link=20 
fib go 176 rc=0 
fib sep 214 rc=0 check=69 ssa=23 insert_rc=7 inline=5 emit=5 assemble=2 link=54 
fib whole 377 rc=0 check=73 ssa=48 insert_rc=35 inline=37 emit=18 assemble=7 link=21 
fib go 167 rc=0 
fib sep 206 rc=0 check=66 ssa=25 insert_rc=5 inline=2 emit=4 assemble=2 link=51 
fib whole 362 rc=0 check=66 ssa=48 insert_rc=35 inline=33 emit=17 assemble=7 link=20 
fib go 170 rc=0 
fib sep 204 rc=0 check=65 ssa=24 insert_rc=4 inline=3 emit=4 assemble=2 link=52 
fib whole 369 rc=0 check=69 ssa=49 insert_rc=35 inline=35 emit=17 assemble=7 link=20 
fib go 224 rc=0 
fib sep 224 rc=0 check=88 ssa=20 insert_rc=2 inline=2 emit=4 assemble=3 link=51 
fib whole 354 rc=0 check=63 ssa=55 insert_rc=36 inline=33 emit=13 assemble=7 link=19 
fib go 160 rc=0 
fib sep 189 rc=0 check=65 ssa=19 insert_rc=4 inline=4 emit=5 assemble=2 link=53 
fib whole 333 rc=0 check=65 ssa=43 insert_rc=35 inline=35 emit=13 assemble=7 link=20 
fib go 152 rc=0 
fib sep 195 rc=0 check=67 ssa=19 insert_rc=4 inline=2 emit=5 assemble=2 link=53 
fib whole 331 rc=0 check=67 ssa=44 insert_rc=36 inline=33 emit=13 assemble=7 link=20 
fib go 157 rc=0 
fib sep 193 rc=0 check=68 ssa=19 insert_rc=3 inline=4 emit=4 assemble=3 link=52 
fib whole 338 rc=0 check=64 ssa=43 insert_rc=37 inline=32 emit=14 assemble=7 link=20 
fib go 154 rc=0 
fib sep 198 rc=0 check=70 ssa=19 insert_rc=2 inline=2 emit=5 assemble=3 link=51 
fib whole 354 rc=0 check=67 ssa=53 insert_rc=34 inline=32 emit=14 assemble=7 link=19 
fib go 158 rc=0 
fib sep 195 rc=0 check=67 ssa=19 insert_rc=4 inline=4 emit=5 assemble=3 link=51 
fib whole 336 rc=0 check=63 ssa=47 insert_rc=37 inline=31 emit=13 assemble=8 link=21 
fib go 158 rc=0 
fib sep 189 rc=0 check=65 ssa=19 insert_rc=2 inline=3 emit=4 assemble=3 link=51 
fib whole 337 rc=0 check=67 ssa=46 insert_rc=30 inline=35 emit=12 assemble=7 link=21 
fib go 162 rc=0 
fib sep 197 rc=0 check=66 ssa=20 insert_rc=4 inline=4 emit=7 assemble=2 link=53 
fib whole 330 rc=0 check=65 ssa=45 insert_rc=32 inline=33 emit=14 assemble=7 link=20 
fib go 161 rc=0 
fib sep 190 rc=0 check=68 ssa=19 insert_rc=5 inline=4 emit=4 assemble=2 link=52 
fib whole 363 rc=0 check=65 ssa=57 insert_rc=34 inline=32 emit=13 assemble=7 link=21 
fib go 161 rc=0 
fib sep 196 rc=0 check=68 ssa=19 insert_rc=6 inline=2 emit=5 assemble=3 link=52 
fib whole 337 rc=0 check=67 ssa=44 insert_rc=35 inline=35 emit=14 assemble=7 link=20 
fib go 162 rc=0 
fib sep 195 rc=0 check=66 ssa=19 insert_rc=4 inline=3 emit=4 assemble=2 link=52 
fib whole 333 rc=0 check=66 ssa=44 insert_rc=36 inline=30 emit=13 assemble=7 link=21 
fib go 161 rc=0 
fib sep 193 rc=0 check=66 ssa=18 insert_rc=2 inline=4 emit=5 assemble=2 link=52 
fib whole 319 rc=0 check=67 ssa=44 insert_rc=30 inline=31 emit=12 assemble=7 link=19 
fib go 159 rc=0 
fib sep 190 rc=0 check=66 ssa=18 insert_rc=3 inline=2 emit=5 assemble=3 link=51 
fib whole 346 rc=0 check=65 ssa=53 insert_rc=32 inline=33 emit=14 assemble=7 link=20 
fib go 173 rc=0 
http sep 447 rc=0 check=118 ssa=51 insert_rc=24 inline=28 emit=13 assemble=3 link=101 
http whole 802 rc=0 check=123 ssa=106 insert_rc=76 inline=77 emit=39 assemble=17 link=68 
http go 473 rc=0 
http sep 439 rc=0 check=119 ssa=44 insert_rc=23 inline=26 emit=13 assemble=3 link=99 
http whole 789 rc=0 check=118 ssa=101 insert_rc=74 inline=78 emit=38 assemble=18 link=68 
http go 395 rc=0 
http sep 450 rc=0 check=120 ssa=44 insert_rc=27 inline=28 emit=11 assemble=3 link=102 
http whole 797 rc=0 check=118 ssa=101 insert_rc=77 inline=80 emit=36 assemble=18 link=68 
http go 382 rc=0 
http sep 455 rc=0 check=124 ssa=44 insert_rc=22 inline=26 emit=15 assemble=3 link=99 
http whole 882 rc=0 check=119 ssa=129 insert_rc=85 inline=83 emit=38 assemble=18 link=68 
http go 391 rc=0 
http sep 443 rc=0 check=119 ssa=44 insert_rc=20 inline=36 emit=13 assemble=3 link=101 
http whole 798 rc=0 check=116 ssa=104 insert_rc=77 inline=80 emit=37 assemble=21 link=68 
http go 390 rc=0 
http sep 466 rc=0 check=124 ssa=43 insert_rc=28 inline=30 emit=12 assemble=3 link=101 
http whole 794 rc=0 check=117 ssa=102 insert_rc=77 inline=77 emit=42 assemble=19 link=67 
http go 383 rc=0 
http sep 464 rc=0 check=123 ssa=49 insert_rc=24 inline=32 emit=13 assemble=3 link=102 
http whole 806 rc=0 check=123 ssa=102 insert_rc=80 inline=74 emit=38 assemble=21 link=68 
http go 480 rc=0 
http sep 444 rc=0 check=120 ssa=44 insert_rc=23 inline=25 emit=11 assemble=3 link=100 
http whole 871 rc=0 check=118 ssa=129 insert_rc=78 inline=80 emit=38 assemble=18 link=69 
http go 388 rc=0 
http sep 452 rc=0 check=119 ssa=43 insert_rc=25 inline=32 emit=13 assemble=3 link=101 
http whole 808 rc=0 check=121 ssa=107 insert_rc=76 inline=78 emit=38 assemble=19 link=68 
http go 390 rc=0 
http sep 600 rc=0 check=120 ssa=69 insert_rc=43 inline=46 emit=72 assemble=3 link=102 
http whole 1717 rc=0 check=119 ssa=103 insert_rc=280 inline=537 emit=98 assemble=20 link=80 
http go 393 rc=0 
http sep 566 rc=0 check=118 ssa=69 insert_rc=37 inline=44 emit=70 assemble=3 link=102 
http whole 796 rc=0 check=117 ssa=102 insert_rc=75 inline=83 emit=41 assemble=17 link=79 
http go 395 rc=0 
http sep 439 rc=0 check=121 ssa=43 insert_rc=19 inline=30 emit=11 assemble=3 link=101 
http whole 881 rc=0 check=120 ssa=128 insert_rc=81 inline=74 emit=36 assemble=19 link=69 
http go 394 rc=0 
http sep 500 rc=0 check=173 ssa=57 insert_rc=20 inline=33 emit=14 assemble=3 link=104 
http whole 779 rc=0 check=120 ssa=104 insert_rc=73 inline=76 emit=36 assemble=19 link=70 
http go 388 rc=0 
http sep 441 rc=0 check=119 ssa=43 insert_rc=24 inline=21 emit=14 assemble=3 link=103 
http whole 877 rc=0 check=125 ssa=104 insert_rc=104 inline=129 emit=36 assemble=18 link=68 
http go 391 rc=0 
http sep 575 rc=0 check=120 ssa=74 insert_rc=39 inline=38 emit=74 assemble=2 link=104 
http whole 788 rc=0 check=117 ssa=107 insert_rc=80 inline=77 emit=38 assemble=18 link=67 
http go 384 rc=0 
http sep 549 rc=0 check=121 ssa=46 insert_rc=47 inline=36 emit=78 assemble=3 link=102 
http whole 886 rc=0 check=122 ssa=130 insert_rc=73 inline=81 emit=38 assemble=17 link=69 
http go 392 rc=0 
http sep 461 rc=0 check=122 ssa=74 insert_rc=17 inline=21 emit=12 assemble=3 link=104 
http whole 782 rc=0 check=119 ssa=103 insert_rc=82 inline=80 emit=36 assemble=18 link=69 
http go 386 rc=0 
http sep 432 rc=0 check=122 ssa=43 insert_rc=20 inline=21 emit=12 assemble=3 link=103 
http whole 914 rc=0 check=122 ssa=103 insert_rc=122 inline=143 emit=38 assemble=18 link=70 
http go 483 rc=0 
http sep 491 rc=0 check=115 ssa=53 insert_rc=21 inline=31 emit=19 assemble=4 link=130 
http whole 789 rc=0 check=117 ssa=102 insert_rc=83 inline=78 emit=38 assemble=18 link=68 
http go 387 rc=0 
http sep 423 rc=0 check=121 ssa=42 insert_rc=24 inline=23 emit=11 assemble=3 link=102 
http whole 895 rc=0 check=118 ssa=138 insert_rc=76 inline=74 emit=40 assemble=18 link=69 
http go 389 rc=0 
```
