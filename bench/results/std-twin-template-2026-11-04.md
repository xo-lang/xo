# std/template twin rows after Lang 1's inline Str and List drops (2026-11-04)

Host 1 (linux/arm64, `--early`, n=6), bench/perf/template at 4fc149d9e (includes 9353f1af9 strdrop and 2deeab68e ordcmp).

| workload | Go ms | Xo ms | ratio | interval |
|---|---|---|---|---|
| parse | 106.6 | 95.11 | 0.892 | 0.871 to 0.91 |
| render | 231.9 | 280.2 | 1.209 | 1.185 to 1.236 |

Before (std-twin rows of 2026-11-03): parse 0.903, render 1.355. Render's remaining cost is the walker (Ex.walk, eval_command) and V dup and drop in the Mac profile (about 20 percent).
