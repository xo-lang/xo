kind: perf
module: prelude
result: fail
commit: 8b0b348a0
source: 675e826b9903565726cf6d0b706c8165dca57478fbb134c2ef46ade915a874bc
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" prelude
measure: 95 cases of 12 twin directories, worst Xo / Go 4.261 over the cases above 5 ms (Proven at or under 1.05), 2m4s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/bytebuf, bench/perf/bytespredicate, bench/perf/bytessearch, bench/perf/bytessplit, bench/perf/bytestransform, bench/perf/mapiter, bench/perf/repeat, bench/perf/sort, bench/perf/strconv, bench/perf/strsplit, bench/perf/trimchars, bench/perf/unicode; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/bytebuf/none (under 5 ms: startup) | 1.5 | 1.1 | 0.730 |
| bench/perf/bytebuf/small_concat | 13.2 | 10.3 | 0.781 |
| bench/perf/bytebuf/small_views | 13.1 | 10.0 | 0.762 |
| bench/perf/bytebuf/large1_concat (under 5 ms: startup) | 4.2 | 2.0 | 0.482 |
| bench/perf/bytebuf/large1_views (under 5 ms: startup) | 4.5 | 1.6 | 0.358 |
| bench/perf/bytebuf/small_buf | 13.3 | 10.5 | 0.792 |
| bench/perf/bytebuf/small_vrec | 13.4 | 10.3 | 0.770 |
| bench/perf/bytebuf/large1_buf (under 5 ms: startup) | 4.3 | 1.6 | 0.368 |
| bench/perf/bytebuf/large64_buf | 127.0 | 28.5 | 0.224 |
| bench/perf/bytebuf/large64_views | 121.0 | 28.4 | 0.235 |
| bench/perf/bytespredicate/index (under 5 ms: startup) | 1.8 | 1.5 | 0.806 |
| bench/perf/bytespredicate/find_where (under 5 ms: startup) | 4.2 | 2.3 | 0.549 |
| bench/perf/bytespredicate/rfind_where (under 5 ms: startup) | 3.9 | 2.1 | 0.541 |
| bench/perf/bytespredicate/contains_where | 9.3 | 5.1 | 0.548 |
| bench/perf/bytespredicate/trim_where (under 5 ms: startup) | 4.7 | 3.0 | 0.646 |
| bench/perf/bytespredicate/trim_start_where (under 5 ms: startup) | 3.9 | 2.6 | 0.672 |
| bench/perf/bytespredicate/trim_end_where (under 5 ms: startup) | 2.3 | 1.6 | 0.682 |
| bench/perf/bytespredicate/split_where | 25.4 | 18.3 | 0.719 |
| bench/perf/bytespredicate/map_runes | 18.6 | 17.1 | 0.917 |
| bench/perf/bytessearch/index (under 5 ms: startup) | 1.9 | 1.5 | 0.805 |
| bench/perf/bytessearch/find_any | 6.3 | 5.0 | 0.788 |
| bench/perf/bytessearch/rfind_any | 7.3 | 6.2 | 0.856 |
| bench/perf/bytessearch/contains_any | 6.4 | 4.9 | 0.759 |
| bench/perf/bytessearch/contains_rune (under 5 ms: startup) | 3.3 | 3.9 | 1.198 |
| bench/perf/bytessearch/cut (under 5 ms: startup) | 3.0 | 3.1 | 1.030 |
| bench/perf/bytessearch/cut_prefix (under 5 ms: startup) | 2.1 | 1.8 | 0.852 |
| bench/perf/bytessearch/cut_suffix (under 5 ms: startup) | 2.1 | 1.9 | 0.879 |
| bench/perf/bytessearch/split_after | 18.4 | 9.0 | 0.490 |
| bench/perf/bytessplit/index (under 5 ms: startup) | 2.1 | 1.8 | 0.848 |
| bench/perf/bytessplit/split | 23.3 | 11.2 | 0.479 |
| bench/perf/bytessplit/split_n | 10.9 | 5.8 | 0.535 |
| bench/perf/bytessplit/fields | 22.7 | 12.9 | 0.566 |
| bench/perf/bytessplit/trim (under 5 ms: startup) | 3.4 | 2.8 | 0.839 |
| bench/perf/bytessplit/affix (under 5 ms: startup) | 2.9 | 3.2 | 1.112 |
| bench/perf/bytestransform/index (under 5 ms: startup) | 2.1 | 1.8 | 0.833 |
| bench/perf/bytestransform/replace | 15.0 | 12.4 | 0.826 |
| bench/perf/bytestransform/replace_n | 10.5 | 7.0 | 0.668 |
| bench/perf/bytestransform/lower | 18.1 | 10.9 | 0.605 |
| bench/perf/bytestransform/upper | 18.1 | 14.3 | 0.792 |
| bench/perf/bytestransform/fold (under 5 ms: startup) | 3.2 | 3.0 | 0.934 |
| bench/perf/bytestransform/repeat | 8.1 | 3.9 | 0.485 |
| bench/perf/bytestransform/lossy | 5.3 | 5.2 | 0.986 |
| bench/perf/bytestransform/valid | 9.4 | 5.1 | 0.542 |
| bench/perf/mapiter/none (under 5 ms: startup) | 1.6 | 1.2 | 0.761 |
| bench/perf/mapiter/walk | 23.0 | 9.0 | 0.390 |
| bench/perf/mapiter/keys | 20.5 | 3.6 | 0.177 |
| bench/perf/repeat/none (under 5 ms: startup) | 1.5 | 1.1 | 0.740 |
| bench/perf/repeat/byte1 | 387.8 | 1246.6 | 3.215 |
| bench/perf/repeat/byte3 | 256.2 | 1091.8 | 4.261 |
| bench/perf/repeat/byte100 | 364.2 | 1177.8 | 3.234 |
| bench/perf/sort/ints | 70.6 | 62.9 | 0.891 |
| bench/perf/sort/strs | 36.2 | 28.5 | 0.788 |
| bench/perf/sort/floats | 80.8 | 80.6 | 0.998 |
| bench/perf/sort/by | 173.1 | 110.3 | 0.637 |
| bench/perf/strconv/atoi | 34.3 | 30.4 | 0.885 |
| bench/perf/strconv/itoa | 110.0 | 63.6 | 0.578 |
| bench/perf/strconv/atof | 131.4 | 92.1 | 0.701 |
| bench/perf/strconv/ftoa | 296.6 | 213.2 | 0.719 |
| bench/perf/strsplit/split | 15.8 | 8.7 | 0.551 |
| bench/perf/strsplit/split_one | 11.7 | 6.0 | 0.516 |
| bench/perf/strsplit/split_multi | 9.8 | 6.6 | 0.674 |
| bench/perf/strsplit/split_absent | 7.3 | 4.8 | 0.657 |
| bench/perf/strsplit/split_empty | 33.2 | 18.0 | 0.542 |
| bench/perf/strsplit/split_after | 11.9 | 7.0 | 0.585 |
| bench/perf/strsplit/lines | 10.2 | 5.9 | 0.572 |
| bench/perf/strsplit/bytes_split | 16.9 | 7.5 | 0.443 |
| bench/perf/strsplit/bytes_fields | 21.7 | 9.4 | 0.432 |
| bench/perf/strsplit/fields | 16.2 | 10.4 | 0.641 |
| bench/perf/strsplit/index (under 5 ms: startup) | 2.0 | 1.8 | 0.907 |
| bench/perf/trimchars/index | 13.7 | 12.8 | 0.932 |
| bench/perf/trimchars/trim | 22.0 | 29.2 | 1.326 |
| bench/perf/trimchars/trim_start | 20.3 | 22.1 | 1.088 |
| bench/perf/trimchars/trim_end | 19.1 | 22.0 | 1.153 |
| bench/perf/trimchars/one | 10.9 | 18.4 | 1.696 |
| bench/perf/trimchars/pair | 35.6 | 24.0 | 0.675 |
| bench/perf/trimchars/abc | 36.7 | 28.9 | 0.789 |
| bench/perf/trimchars/uni | 39.7 | 37.6 | 0.947 |
| bench/perf/unicode/lower | 111.1 | 12.1 | 0.109 |
| bench/perf/unicode/upper | 121.6 | 12.1 | 0.100 |
| bench/perf/unicode/lower_u | 248.8 | 143.1 | 0.575 |
| bench/perf/unicode/upper_u | 251.3 | 151.5 | 0.603 |
| bench/perf/unicode/lower_az | 26.1 | 5.2 | 0.199 |
| bench/perf/unicode/upper_az | 30.0 | 5.0 | 0.168 |
| bench/perf/unicode/eqlower | 720.7 | 297.1 | 0.412 |
| bench/perf/unicode/classify | 19.6 | 7.0 | 0.358 |
| bench/perf/unicode/runecase | 13.3 | 10.5 | 0.792 |
| bench/perf/unicode/classes | 48.2 | 20.5 | 0.425 |
| bench/perf/unicode/latin1 (under 5 ms: startup) | 3.9 | 3.4 | 0.874 |
| bench/perf/unicode/title | 10.2 | 9.2 | 0.903 |
| bench/perf/unicode/utf8len (under 5 ms: startup) | 4.3 | 3.9 | 0.893 |
| bench/perf/unicode/quote | 15.6 | 6.0 | 0.385 |
| bench/perf/unicode/indexfunc | 347.4 | 294.0 | 0.846 |
| bench/perf/unicode/trimfunc | 7.1 | 3.7 | 0.519 |
| bench/perf/unicode/fieldsfunc | 33.6 | 11.7 | 0.349 |
| bench/perf/unicode/mapfunc | 209.6 | 116.9 | 0.558 |
