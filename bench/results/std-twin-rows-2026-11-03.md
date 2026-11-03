# Timing rows against Go for csv, xml, compress, archive, mime and mail (2026-11-03)

The eight fuzzed modules (decision 0461, performance axis) need a committed result against Go's package.
Twins: bench/perf/csv and bench/perf/xml (new; `go run ./scripts/twincheck` agrees on every workload),
bench/perf/compress, archive, mime and mail (2026-10-26/27). Host 1 (linux/arm64, 12 CPUs, exclusive lease,
load 0.9), main at b934b1f97 plus the xml change below, `xo bench --ab --rounds=15`, A the Go twin
(go build), B the Xo program (`xo build --release`, native arm64); ratios are Xo / Go, with the 95% interval.
`none` is the setup alone. Peak memory is the Xo / Go ratio of max RSS.

| workload | Go wall | Xo wall | wall | interval | peak memory |
|---|---|---|---|---|---|
| csv none | 3.028 ms | 2.356 ms | 0.778 | 0.741 to 0.8 | 0.961 |
| csv parse | 35.98 ms | 33.24 ms | 0.924 | 0.907 to 0.949 | 0.573 |
| csv format | 36.92 ms | 37.31 ms | 1.011 | 0.979 to 1.067 | 0.599 |
| xml none | 3.238 ms | 3.003 ms | 0.927 | 0.879 to 0.954 | 0.939 |
| xml tokens, first run | 215.8 ms | 339.2 ms | 1.572 | 1.567 to 1.587 | 3.002 |
| xml tokens, after text_entities | 213.9 ms | 245.5 ms | 1.148 | 1.134 to 1.152 | 3.02 |
| xml tree, first run | 302.9 ms | 377 ms | 1.245 | 1.233 to 1.256 | 2.159 |
| xml tree, after text_entities | 301.3 ms | 284.6 ms | 0.945 | 0.937 to 0.955 | 2.137 |
| compress none | 4.372 ms | 2.956 ms | 0.676 | 0.656 to 0.692 | 0.777 |
| compress gzip | 513.8 ms | 492.4 ms | 0.958 | 0.957 to 0.962 | 0.581 |
| compress gzip1 | 100.5 ms | 67.3 ms | 0.669 | 0.654 to 0.676 | 0.553 |
| compress gunzip | 91.8 ms | 84.54 ms | 0.921 | 0.913 to 0.926 | 0.575 |
| compress deflate | 511.6 ms | 490.6 ms | 0.959 | 0.956 to 0.963 | 0.667 |
| compress inflate | 90.23 ms | 82.01 ms | 0.909 | 0.901 to 0.918 | 0.576 |
| archive none | 1.005 ms | 0.54 ms | 0.538 | 0.515 to 0.56 | 0.932 |
| archive tar | 15.03 ms | 16.26 ms | 1.082 | 1.05 to 1.091 | 0.573 |
| archive untar | 25.4 ms | 7.686 ms | 0.303 | 0.296 to 0.311 | 0.552 |
| archive zip | 489.7 ms | 661.9 ms | 1.352 | 1.343 to 1.36 | 0.552 |
| archive unzip | 89.17 ms | 89.11 ms | 0.999 | 0.988 to 1.007 | 0.538 |
| mime none | 1.277 ms | 0.882 ms | 0.691 | 0.674 to 0.724 | 1.0 |
| mime parse | 25.5 ms | 27.94 ms | 1.096 | 1.075 to 1.114 | 0.627 |
| mime format | 49.27 ms | 64.7 ms | 1.313 | 1.284 to 1.336 | 0.626 |
| mime ext | 4.046 ms | 3.548 ms | 0.877 | 0.854 to 0.903 | 0.983 |
| mime detect | 18.15 ms | 16.11 ms | 0.888 | 0.883 to 0.902 | 1.0 |
| mail none | 1.051 ms | 0.701 ms | 0.666 | 0.631 to 0.686 | 0.913 |
| mail addr | 35.54 ms | 21.98 ms | 0.619 | 0.612 to 0.628 | 0.543 |
| mail list | 118.6 ms | 66.79 ms | 0.563 | 0.545 to 0.571 | 0.537 |
| mail display | 11.74 ms | 13.45 ms | 1.145 | 1.123 to 1.166 | 0.55 |
| mail msg | 131 ms | 59.07 ms | 0.451 | 0.444 to 0.457 | 0.532 |
| mail date | 122.2 ms | 23.6 ms | 0.193 | 0.188 to 0.197 | 0.541 |

Per module, the weakest workload: csv 1.01 (format), xml 1.15 (tokens), compress 0.96 (gzip), archive 1.35
(zip), mime 1.31 (format), mail 1.15 (display). Only xml tokens was over the 1.5x line (1.57); the cause was
character data holding an entity (`&amp;`): any entity left the fast path for the byte by byte one
(Dec.getc 21% and Dec.text 14% flat in an XO_PROF=cpu profile of 80 passes on the Mac). std/native/xml.xo
`text_entities` now reads the five standard entities in the fast path (std/changes/xml.md).

Peak memory: xml is 3.0x (tokens) and 2.1x (tree) of Go's because `xml.tokens` returns every token and `parse`
holds the whole tree, where Go's Decoder streams tokens and Unmarshal fills structs; csv, compress, archive,
mime and mail are 0.5 to 1.0 of Go's.

Left over the Go twin: archive zip 1.35, mime format 1.31, xml tokens 1.15, mail display 1.15, archive tar
1.08, mime parse 1.10. None is over 1.5x.
