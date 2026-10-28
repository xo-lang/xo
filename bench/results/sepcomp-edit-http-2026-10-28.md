# Separately compiled edit builds (decision 0388 stage 3): the HTTP service against xo 0.33.0

2026-10-28, darwin/arm64 (the development Mac, 24 GB), measure class (quiet machine, load 4 at start). ~/xo-lead/notes/lang1/ab033.sh http: A the 0.33.0 release xo (seed 0.33.0-04280451, its own std), B the xo of main at 2e71c909f with XO_SEPCOMP=1 (std's functions externs answered by std's conc1 facts, the program being concurrent; std linked from its conc1 store, both from `xo dev stdcache --conc=1`). Each side warmed with three builds in its own backend cache, then 20 interleaved edits of bench/perf/http/xo/api.xo (the text of an error message), wall time from outside the process (perl Time::HiRes around waitpid).

| | median ms | mean ms | min ms | max ms |
|---|---|---|---|---|
| 0.33.0 (whole program) | 579.5 | 576.9 | 542 | 614 |
| separate (XO_SEPCOMP) | 225.0 | 227.2 | 215 | 245 |

Phase means (ms, --phase-times):

| phase | 0.33.0 | separate |
|---|---|---|
| check | 56.0 | 57.8 |
| ssa | 93.9 | 27.9 |
| insert_rc | 59.5 | 10.1 |
| inline | 60.5 | 11.8 |
| emit | 21.9 | 7.7 |
| assemble | 7.3 | 1.2 |
| link | 41.2 | 43.5 |

40 of 40 builds succeeded. The milestone 0.34.0 gate is an HTTP edit at most 500 ms. The rest of the separate build's time (about 67 ms of the mean, beyond the phases listed) is parsing, process start and exit, the keys, the cache reads and stores; the link is now the largest phase.

Raw:

```
http A 542 rc=0 check=57 ssa=82 insert_rc=55 inline=56 emit=21 assemble=7 link=40 
http B 222 rc=0 check=57 ssa=26 insert_rc=8 inline=10 emit=7 assemble=1 link=43 
http A 573 rc=0 check=56 ssa=106 insert_rc=56 inline=62 emit=22 assemble=6 link=41 
http B 219 rc=0 check=54 ssa=26 insert_rc=10 inline=11 emit=9 assemble=1 link=45 
http A 594 rc=0 check=55 ssa=91 insert_rc=67 inline=65 emit=23 assemble=7 link=42 
http B 245 rc=0 check=65 ssa=29 insert_rc=11 inline=12 emit=7 assemble=1 link=45 
http A 608 rc=0 check=59 ssa=90 insert_rc=58 inline=61 emit=21 assemble=9 link=41 
http B 226 rc=0 check=57 ssa=28 insert_rc=10 inline=12 emit=9 assemble=1 link=42 
http A 555 rc=0 check=56 ssa=88 insert_rc=58 inline=59 emit=21 assemble=9 link=42 
http B 225 rc=0 check=57 ssa=28 insert_rc=11 inline=12 emit=7 assemble=1 link=44 
http A 575 rc=0 check=57 ssa=112 insert_rc=57 inline=59 emit=22 assemble=7 link=41 
http B 229 rc=0 check=57 ssa=28 insert_rc=10 inline=11 emit=9 assemble=1 link=43 
http A 567 rc=0 check=56 ssa=89 insert_rc=61 inline=61 emit=22 assemble=7 link=41 
http B 227 rc=0 check=57 ssa=29 insert_rc=9 inline=11 emit=7 assemble=2 link=45 
http A 597 rc=0 check=55 ssa=88 insert_rc=58 inline=61 emit=21 assemble=8 link=43 
http B 228 rc=0 check=56 ssa=27 insert_rc=11 inline=10 emit=7 assemble=1 link=42 
http A 556 rc=0 check=56 ssa=88 insert_rc=58 inline=56 emit=22 assemble=11 link=41 
http B 222 rc=0 check=58 ssa=28 insert_rc=13 inline=13 emit=7 assemble=1 link=43 
http A 585 rc=0 check=55 ssa=112 insert_rc=56 inline=60 emit=25 assemble=7 link=40 
http B 226 rc=0 check=57 ssa=28 insert_rc=9 inline=13 emit=8 assemble=2 link=43 
http A 547 rc=0 check=55 ssa=86 insert_rc=58 inline=61 emit=21 assemble=7 link=41 
http B 215 rc=0 check=56 ssa=27 insert_rc=8 inline=11 emit=7 assemble=1 link=42 
http A 590 rc=0 check=55 ssa=89 insert_rc=59 inline=57 emit=21 assemble=7 link=41 
http B 225 rc=0 check=55 ssa=26 insert_rc=9 inline=13 emit=12 assemble=1 link=43 
http A 575 rc=0 check=55 ssa=87 insert_rc=66 inline=61 emit=23 assemble=11 link=42 
http B 239 rc=0 check=59 ssa=31 insert_rc=13 inline=13 emit=7 assemble=1 link=45 
http A 614 rc=0 check=56 ssa=116 insert_rc=67 inline=67 emit=23 assemble=7 link=41 
http B 222 rc=0 check=57 ssa=28 insert_rc=10 inline=11 emit=7 assemble=2 link=44 
http A 557 rc=0 check=55 ssa=89 insert_rc=58 inline=58 emit=21 assemble=6 link=41 
http B 223 rc=0 check=55 ssa=29 insert_rc=9 inline=11 emit=7 assemble=1 link=42 
http A 586 rc=0 check=55 ssa=86 insert_rc=57 inline=58 emit=23 assemble=7 link=41 
http B 223 rc=0 check=57 ssa=28 insert_rc=10 inline=11 emit=7 assemble=1 link=43 
http A 553 rc=0 check=57 ssa=88 insert_rc=58 inline=63 emit=21 assemble=6 link=41 
http B 223 rc=0 check=57 ssa=27 insert_rc=11 inline=12 emit=7 assemble=1 link=44 
http A 587 rc=0 check=57 ssa=114 insert_rc=59 inline=59 emit=21 assemble=6 link=42 
http B 245 rc=0 check=71 ssa=29 insert_rc=12 inline=14 emit=8 assemble=1 link=44 
http A 584 rc=0 check=56 ssa=90 insert_rc=69 inline=65 emit=22 assemble=6 link=41 
http B 240 rc=0 check=58 ssa=28 insert_rc=9 inline=13 emit=7 assemble=2 link=46 
http A 593 rc=0 check=57 ssa=87 insert_rc=56 inline=60 emit=23 assemble=6 link=41 
http B 221 rc=0 check=56 ssa=27 insert_rc=9 inline=13 emit=7 assemble=2 link=42 
```
