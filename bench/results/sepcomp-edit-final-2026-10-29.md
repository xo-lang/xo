# Edit builds at the 0.34 cut: fib and the HTTP service against 0.33.0 and Go (2026-10-29)

darwin/arm64 (the development Mac, 24 GB), measure class (quiet machine), ~/xo-lead/notes/lang1/finalab2.sh: A the 0.33.0 release xo (seed 0.33.0, its own std), B the xo of main c462dd4e1 (the cut's blockers in: separate compilation by default, stage 2's std record by default 4c1c15e05, the linux store fix 661d243e9) with no variable set but XO_CACHE_STATS=1 (to show the path each build took; its counting can only slow B), G `go build` (go1.26.1) of the Go twin. std's variants built first with `xo dev stdcache --conc=0` and `--conc=1` (a fresh xo builds them in the background after its first build otherwise); each side warmed three times in its own cache and directory, then 20 interleaved edits: fib, the fib argument in main.xo (the `fib(40)` constant in bench/perf/llvm/go/main.go for Go); HTTP, an error message's text (api.xo; handlers.go for Go). Wall time from outside the process (one perl forking and waiting).

| edit | 0.33.0 median (mean) | 0.34 cut median (mean) | go build median (mean) |
|---|---|---|---|
| fib | 230.8 (231.1) ms | 70.2 (71.1) ms | 87.7 (94.3) ms |
| HTTP | 570.5 (565.7) ms | 156.0 (157.2) ms | 245.5 (248.6) ms |

Ranges: fib A 218.7 to 247.2, B 67.5 to 88.8, G 85.5 to 110.2; HTTP A 532.2 to 628.0, B 152.6 to 171.1, G 241.3 to 273.3. 120 of 120 builds succeeded. Every counted B build took the separate path: `cache std ... reused` (std's variants) and `cache stdx record loaded` (stage 2's record) in 20 of 20 for fib and 20 of 20 for HTTP. fib's output is the same from A's and B's binaries (fib 121393).

Phase means (ms, --phase-times):

| phase | fib 0.33.0 | fib cut | HTTP 0.33.0 | HTTP cut |
|---|---|---|---|---|
| check | 30.5 | 17.4 | 54.9 | 25.1 |
| ssa | 39.0 | 6.4 | 91.8 | 13.3 |
| insert_rc | 24.2 | 2.0 | 57.1 | 9.2 |
| inline | 24.8 | 2.0 | 59.4 | 10.0 |
| emit | 7.5 | 1.0 | 21.8 | 7.6 |
| assemble | 2.0 | 1.1 | 7.5 | 1.1 |
| link | 11.2 | 16.6 | 40.5 | 38.0 |

The milestone 0.34.0 gates were a fib edit at most 230 ms and an HTTP edit at most 500 ms. The link is now the largest phase of both (fib 16.6 of 70.2 ms, HTTP 38.0 of 156.0).

Raw:

```
fib A 232.5 rc=0 check=32 ssa=36 insert_rc=23 inline=25 emit=8 assemble=2 link=12 
fib B 88.8 rc=0 std=2 stdx=1 check=24 ssa=6 insert_rc=2 inline=2 emit=1 assemble=1 link=16 
fib G 110.2 rc=0 
fib A 244.5 rc=0 check=32 ssa=45 insert_rc=26 inline=25 emit=8 assemble=2 link=12 
fib B 72.5 rc=0 std=2 stdx=1 check=17 ssa=7 insert_rc=2 inline=2 emit=1 assemble=1 link=17 
fib G 105.4 rc=0 
fib A 232.0 rc=0 check=30 ssa=36 insert_rc=25 inline=25 emit=9 assemble=2 link=11 
fib B 75.0 rc=0 std=2 stdx=1 check=18 ssa=7 insert_rc=2 inline=2 emit=1 assemble=1 link=17 
fib G 110.2 rc=0 
fib A 247.2 rc=0 check=31 ssa=39 insert_rc=24 inline=23 emit=9 assemble=2 link=11 
fib B 70.5 rc=0 std=2 stdx=1 check=17 ssa=7 insert_rc=2 inline=2 emit=1 assemble=1 link=16 
fib G 106.7 rc=0 
fib A 244.2 rc=0 check=32 ssa=37 insert_rc=28 inline=28 emit=8 assemble=2 link=11 
fib B 71.0 rc=0 std=2 stdx=1 check=17 ssa=7 insert_rc=2 inline=2 emit=1 assemble=1 link=16 
fib G 106.3 rc=0 
fib A 240.9 rc=0 check=30 ssa=45 insert_rc=23 inline=26 emit=8 assemble=2 link=11 
fib B 73.0 rc=0 std=2 stdx=1 check=17 ssa=7 insert_rc=2 inline=1 emit=1 assemble=1 link=17 
fib G 105.2 rc=0 
fib A 236.0 rc=0 check=31 ssa=37 insert_rc=24 inline=25 emit=8 assemble=2 link=12 
fib B 69.7 rc=0 std=2 stdx=1 check=17 ssa=7 insert_rc=2 inline=2 emit=1 assemble=1 link=17 
fib G 108.0 rc=0 
fib A 229.2 rc=0 check=29 ssa=36 insert_rc=24 inline=25 emit=7 assemble=2 link=11 
fib B 67.5 rc=0 std=2 stdx=1 check=17 ssa=7 insert_rc=2 inline=2 emit=1 assemble=1 link=16 
fib G 86.7 rc=0 
fib A 223.1 rc=0 check=30 ssa=36 insert_rc=25 inline=25 emit=7 assemble=2 link=12 
fib B 71.2 rc=0 std=2 stdx=1 check=17 ssa=6 insert_rc=2 inline=2 emit=1 assemble=1 link=16 
fib G 85.5 rc=0 
fib A 227.9 rc=0 check=30 ssa=44 insert_rc=23 inline=24 emit=7 assemble=2 link=11 
fib B 68.3 rc=0 std=2 stdx=1 check=17 ssa=6 insert_rc=2 inline=2 emit=1 assemble=1 link=17 
fib G 87.7 rc=0 
fib A 220.6 rc=0 check=30 ssa=37 insert_rc=24 inline=23 emit=7 assemble=2 link=11 
fib B 69.9 rc=0 std=2 stdx=1 check=17 ssa=6 insert_rc=2 inline=2 emit=1 assemble=2 link=17 
fib G 87.7 rc=0 
fib A 233.3 rc=0 check=30 ssa=38 insert_rc=25 inline=25 emit=7 assemble=2 link=11 
fib B 67.7 rc=0 std=2 stdx=1 check=16 ssa=6 insert_rc=2 inline=2 emit=1 assemble=1 link=17 
fib G 87.7 rc=0 
fib A 218.7 rc=0 check=30 ssa=36 insert_rc=25 inline=23 emit=7 assemble=2 link=11 
fib B 69.4 rc=0 std=2 stdx=1 check=17 ssa=6 insert_rc=2 inline=2 emit=1 assemble=1 link=17 
fib G 86.6 rc=0 
fib A 223.7 rc=0 check=29 ssa=44 insert_rc=22 inline=24 emit=7 assemble=2 link=11 
fib B 70.4 rc=0 std=2 stdx=1 check=17 ssa=6 insert_rc=2 inline=2 emit=1 assemble=1 link=16 
fib G 86.5 rc=0 
fib A 219.9 rc=0 check=30 ssa=36 insert_rc=24 inline=25 emit=8 assemble=2 link=11 
fib B 68.7 rc=0 std=2 stdx=1 check=17 ssa=6 insert_rc=2 inline=2 emit=1 assemble=1 link=16 
fib G 88.0 rc=0 
fib A 231.6 rc=0 check=30 ssa=36 insert_rc=23 inline=24 emit=7 assemble=2 link=11 
fib B 68.5 rc=0 std=2 stdx=1 check=17 ssa=6 insert_rc=2 inline=2 emit=1 assemble=1 link=17 
fib G 87.7 rc=0 
fib A 222.6 rc=0 check=31 ssa=36 insert_rc=23 inline=26 emit=7 assemble=2 link=11 
fib B 69.5 rc=0 std=2 stdx=1 check=17 ssa=6 insert_rc=2 inline=3 emit=1 assemble=1 link=17 
fib G 89.0 rc=0 
fib A 230.0 rc=0 check=30 ssa=44 insert_rc=25 inline=25 emit=7 assemble=2 link=11 
fib B 70.6 rc=0 std=2 stdx=1 check=17 ssa=6 insert_rc=2 inline=2 emit=1 assemble=1 link=17 
fib G 87.7 rc=0 
fib A 227.3 rc=0 check=31 ssa=41 insert_rc=24 inline=25 emit=7 assemble=2 link=11 
fib B 68.4 rc=0 std=2 stdx=1 check=17 ssa=6 insert_rc=2 inline=2 emit=1 assemble=1 link=17 
fib G 87.7 rc=0 
fib A 236.0 rc=0 check=32 ssa=40 insert_rc=24 inline=24 emit=7 assemble=2 link=11 
fib B 71.9 rc=0 std=2 stdx=1 check=17 ssa=7 insert_rc=2 inline=2 emit=1 assemble=1 link=17 
fib G 86.4 rc=0 
http A 532.2 rc=0 check=56 ssa=80 insert_rc=56 inline=57 emit=21 assemble=6 link=40 
http B 171.1 rc=0 std=2 stdx=1 check=37 ssa=16 insert_rc=8 inline=8 emit=7 assemble=1 link=37 
http G 247.1 rc=0 
http A 628.0 rc=0 check=58 ssa=113 insert_rc=67 inline=65 emit=26 assemble=7 link=41 
http B 158.1 rc=0 std=2 stdx=1 check=25 ssa=14 insert_rc=9 inline=11 emit=8 assemble=1 link=38 
http G 273.3 rc=0 
http A 579.5 rc=0 check=58 ssa=89 insert_rc=64 inline=63 emit=21 assemble=11 link=41 
http B 155.7 rc=0 std=2 stdx=1 check=24 ssa=13 insert_rc=11 inline=10 emit=8 assemble=1 link=37 
http G 249.8 rc=0 
http A 576.6 rc=0 check=55 ssa=86 insert_rc=54 inline=59 emit=21 assemble=6 link=40 
http B 153.9 rc=0 std=2 stdx=1 check=24 ssa=13 insert_rc=10 inline=10 emit=7 assemble=1 link=38 
http G 244.9 rc=0 
http A 549.1 rc=0 check=56 ssa=87 insert_rc=55 inline=58 emit=21 assemble=7 link=41 
http B 156.3 rc=0 std=2 stdx=1 check=24 ssa=13 insert_rc=9 inline=9 emit=7 assemble=1 link=38 
http G 258.5 rc=0 
http A 559.3 rc=0 check=53 ssa=109 insert_rc=54 inline=59 emit=21 assemble=7 link=41 
http B 154.7 rc=0 std=2 stdx=1 check=24 ssa=14 insert_rc=10 inline=9 emit=8 assemble=1 link=38 
http G 241.3 rc=0 
http A 541.0 rc=0 check=54 ssa=86 insert_rc=57 inline=60 emit=21 assemble=7 link=41 
http B 152.8 rc=0 std=2 stdx=1 check=24 ssa=13 insert_rc=8 inline=9 emit=8 assemble=1 link=37 
http G 247.1 rc=0 
http A 589.0 rc=0 check=53 ssa=87 insert_rc=54 inline=57 emit=22 assemble=7 link=40 
http B 162.9 rc=0 std=2 stdx=1 check=24 ssa=13 insert_rc=10 inline=11 emit=9 assemble=1 link=39 
http G 254.1 rc=0 
http A 564.0 rc=0 check=55 ssa=85 insert_rc=59 inline=62 emit=21 assemble=7 link=41 
http B 156.5 rc=0 std=2 stdx=1 check=25 ssa=13 insert_rc=8 inline=12 emit=7 assemble=2 link=38 
http G 245.4 rc=0 
http A 571.8 rc=0 check=55 ssa=109 insert_rc=57 inline=58 emit=21 assemble=7 link=40 
http B 156.5 rc=0 std=2 stdx=1 check=25 ssa=14 insert_rc=8 inline=10 emit=8 assemble=1 link=38 
http G 241.8 rc=0 
http A 555.7 rc=0 check=55 ssa=86 insert_rc=56 inline=59 emit=23 assemble=7 link=41 
http B 153.1 rc=0 std=2 stdx=1 check=24 ssa=13 insert_rc=10 inline=11 emit=7 assemble=2 link=38 
http G 242.5 rc=0 
http A 578.3 rc=0 check=54 ssa=86 insert_rc=55 inline=59 emit=21 assemble=7 link=40 
http B 163.1 rc=0 std=2 stdx=1 check=26 ssa=13 insert_rc=8 inline=12 emit=8 assemble=1 link=38 
http G 241.4 rc=0 
http A 572.7 rc=0 check=54 ssa=87 insert_rc=67 inline=64 emit=23 assemble=8 link=42 
http B 157.2 rc=0 std=2 stdx=1 check=25 ssa=13 insert_rc=10 inline=11 emit=8 assemble=1 link=39 
http G 258.8 rc=0 
http A 574.8 rc=0 check=54 ssa=109 insert_rc=56 inline=58 emit=23 assemble=17 link=40 
http B 154.3 rc=0 std=2 stdx=1 check=24 ssa=14 insert_rc=9 inline=10 emit=7 assemble=1 link=38 
http G 243.3 rc=0 
http A 538.0 rc=0 check=56 ssa=85 insert_rc=54 inline=59 emit=22 assemble=7 link=40 
http B 153.6 rc=0 std=2 stdx=1 check=25 ssa=13 insert_rc=8 inline=10 emit=7 assemble=1 link=38 
http G 259.2 rc=0 
http A 582.9 rc=0 check=55 ssa=86 insert_rc=56 inline=58 emit=21 assemble=6 link=40 
http B 166.8 rc=0 std=2 stdx=1 check=26 ssa=14 insert_rc=12 inline=9 emit=8 assemble=2 link=40 
http G 243.9 rc=0 
http A 533.8 rc=0 check=55 ssa=86 insert_rc=55 inline=56 emit=21 assemble=7 link=40 
http B 158.2 rc=0 std=2 stdx=1 check=24 ssa=13 insert_rc=10 inline=9 emit=7 assemble=1 link=38 
http G 247.5 rc=0 
http A 569.2 rc=0 check=54 ssa=109 insert_rc=58 inline=58 emit=21 assemble=6 link=41 
http B 152.6 rc=0 std=2 stdx=1 check=24 ssa=13 insert_rc=10 inline=9 emit=7 assemble=1 link=38 
http G 241.7 rc=0 
http A 540.5 rc=0 check=54 ssa=86 insert_rc=54 inline=57 emit=23 assemble=7 link=41 
http B 153.2 rc=0 std=2 stdx=1 check=25 ssa=12 insert_rc=8 inline=11 emit=8 assemble=1 link=37 
http G 245.6 rc=0 
http A 578.4 rc=0 check=54 ssa=86 insert_rc=55 inline=61 emit=21 assemble=6 link=40 
http B 152.7 rc=0 std=2 stdx=1 check=24 ssa=13 insert_rc=8 inline=9 emit=8 assemble=1 link=37 
http G 244.5 rc=0 
```
