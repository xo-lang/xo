# Separately compiled edit builds (decision 0388 stage 3): llvm-fib against xo 0.33.0

2026-10-28, darwin/arm64 (the development Mac, 24 GB), measure class (quiet machine). ~/xo-lead/notes/lang1/ab033.sh fib: A the 0.33.0 release xo (seed 0.33.0-04280451, its own std), B the xo of branch lang1-0388-s3c with XO_SEPCOMP=1 (std's functions externs answered by std's conc0 facts; std linked from its store, both from `xo dev stdcache --conc=0`). Each side warmed with three builds in its own backend cache, then 20 interleaved edits of bench/perf/llvm/xo/fib/main.xo (the fib argument), wall time from outside the process (perl Time::HiRes).

| | median ms | mean ms |
|---|---|---|
| 0.33.0 (whole program) | 229.0 | 227.5 |
| separate (XO_SEPCOMP) | 111.5 | 112.5 |

Phase means (ms, --phase-times):

| phase | 0.33.0 | separate |
|---|---|---|
| check | 29.9 | 31.6 |
| ssa | 37.8 | 15.7 |
| insert_rc | 23.0 | 2.4 |
| inline | 23.4 | 3.0 |
| emit | 7.4 | 1.0 |
| assemble | 2.0 | 1.0 |
| link | 11.1 | 17.2 |

40 of 40 builds succeeded; fib's output is the same from both binaries. The milestone 0.34.0 gate is a fib edit at most 230 ms; Go's twin of the edit (an appended function): 100 ms (bench/results/release-vs-go-v0.33.0.md, the llvm row). The time outside the phases (about 40 ms of the separate build's) is process start and exit, the keys, the cache reads and stores.

Raw:

```
fib A 225 rc=0 check=30 ssa=34 insert_rc=21 inline=22 emit=8 assemble=2 link=11 
fib B 116 rc=0 check=33 ssa=16 insert_rc=2 inline=3 emit=1 assemble=1 link=17 
fib A 236 rc=0 check=30 ssa=42 insert_rc=23 inline=23 emit=8 assemble=2 link=11 
fib B 114 rc=0 check=32 ssa=17 insert_rc=2 inline=3 emit=1 assemble=1 link=17 
fib A 229 rc=0 check=30 ssa=36 insert_rc=22 inline=23 emit=8 assemble=2 link=12 
fib B 117 rc=0 check=31 ssa=18 insert_rc=4 inline=3 emit=1 assemble=1 link=17 
fib A 243 rc=0 check=30 ssa=37 insert_rc=24 inline=24 emit=8 assemble=2 link=11 
fib B 117 rc=0 check=31 ssa=18 insert_rc=2 inline=3 emit=1 assemble=1 link=18 
fib A 229 rc=0 check=29 ssa=36 insert_rc=23 inline=25 emit=8 assemble=2 link=11 
fib B 115 rc=0 check=31 ssa=18 insert_rc=2 inline=3 emit=1 assemble=1 link=17 
fib A 237 rc=0 check=30 ssa=45 insert_rc=21 inline=22 emit=9 assemble=2 link=11 
fib B 115 rc=0 check=31 ssa=17 insert_rc=2 inline=3 emit=1 assemble=1 link=17 
fib A 236 rc=0 check=30 ssa=38 insert_rc=24 inline=23 emit=8 assemble=2 link=11 
fib B 118 rc=0 check=31 ssa=18 insert_rc=3 inline=3 emit=1 assemble=1 link=18 
fib A 231 rc=0 check=30 ssa=36 insert_rc=23 inline=24 emit=7 assemble=2 link=11 
fib B 111 rc=0 check=31 ssa=15 insert_rc=2 inline=3 emit=1 assemble=1 link=17 
fib A 220 rc=0 check=30 ssa=35 insert_rc=22 inline=23 emit=7 assemble=2 link=11 
fib B 110 rc=0 check=31 ssa=15 insert_rc=4 inline=3 emit=1 assemble=1 link=18 
fib A 226 rc=0 check=29 ssa=44 insert_rc=22 inline=24 emit=7 assemble=2 link=11 
fib B 108 rc=0 check=32 ssa=14 insert_rc=4 inline=3 emit=1 assemble=1 link=17 
fib A 217 rc=0 check=31 ssa=36 insert_rc=21 inline=23 emit=7 assemble=2 link=11 
fib B 111 rc=0 check=31 ssa=15 insert_rc=2 inline=3 emit=1 assemble=1 link=17 
fib A 232 rc=0 check=31 ssa=35 insert_rc=24 inline=25 emit=7 assemble=2 link=11 
fib B 113 rc=0 check=33 ssa=15 insert_rc=2 inline=3 emit=1 assemble=1 link=17 
fib A 219 rc=0 check=29 ssa=35 insert_rc=24 inline=22 emit=7 assemble=2 link=11 
fib B 108 rc=0 check=31 ssa=15 insert_rc=2 inline=3 emit=1 assemble=1 link=17 
fib A 229 rc=0 check=30 ssa=44 insert_rc=23 inline=25 emit=7 assemble=2 link=11 
fib B 105 rc=0 check=31 ssa=14 insert_rc=2 inline=3 emit=1 assemble=1 link=17 
fib A 216 rc=0 check=30 ssa=35 insert_rc=23 inline=23 emit=7 assemble=2 link=11 
fib B 111 rc=0 check=31 ssa=15 insert_rc=2 inline=3 emit=1 assemble=1 link=17 
fib A 230 rc=0 check=30 ssa=36 insert_rc=24 inline=23 emit=7 assemble=2 link=11 
fib B 110 rc=0 check=33 ssa=15 insert_rc=2 inline=3 emit=1 assemble=1 link=17 
fib A 222 rc=0 check=30 ssa=36 insert_rc=25 inline=23 emit=7 assemble=2 link=11 
fib B 112 rc=0 check=33 ssa=15 insert_rc=2 inline=3 emit=1 assemble=1 link=17 
fib A 226 rc=0 check=30 ssa=44 insert_rc=25 inline=22 emit=7 assemble=2 link=11 
fib B 111 rc=0 check=31 ssa=14 insert_rc=2 inline=3 emit=1 assemble=1 link=18 
fib A 218 rc=0 check=30 ssa=36 insert_rc=23 inline=23 emit=7 assemble=2 link=11 
fib B 116 rc=0 check=31 ssa=15 insert_rc=2 inline=3 emit=1 assemble=1 link=17 
fib A 229 rc=0 check=30 ssa=35 insert_rc=23 inline=25 emit=7 assemble=2 link=11 
fib B 111 rc=0 check=32 ssa=15 insert_rc=2 inline=3 emit=1 assemble=1 link=18 
```
