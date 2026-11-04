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

linux/arm64 host 2 (18 CPUs, load 0.95 at start), 7 rounds, Go first, Xo / Go
after the change: text 98.33 / 62.9 = 0.640 (0.636-0.642), json 97.09 /
106.2 = 1.094 (1.08-1.113), filtered 7.151 / 10.56 = 1.477 (the filtered
path writes nothing; its cost is the Map built at the call site). Before
the change, on host 1 (n=6-7): text 1.255, json 1.818, filtered 1.758: a
different host, so the pairing is indicative; host 1 confirms later.
