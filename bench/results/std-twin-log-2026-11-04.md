# std/log: one write per record (2026-11-04)

Stdio.eprintln made two write system calls per line (the text, then "\n");
Go's slog makes one. XO_PROF of bench/perf/log `text` (3,000,000 records,
Mac): 87% of the time in `write`.

Mac (darwin/arm64, release build, stderr to /dev/null, 100,000 `text`
records, 6 runs each, wall seconds): before 0.189 0.190 0.190 0.191 0.196
0.279 (median 0.1905), after 0.117 0.117 0.118 0.118 0.120 0.204 (median
0.118): 0.62 of the time. A Mac measurement only: host 1 dropped its
connection during the linux/arm64 run (twice, 2026-11-04); the row follows
when it is back.
