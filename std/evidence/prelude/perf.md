kind: perf
module: prelude
result: pass
commit: 80d5dfdff
source: 7b90cc9e305fb130ba8e9437223033cef484dad4695db0687d55ee072654af76
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" prelude
measure: 107 cases of 13 twin directories, worst Xo / Go 1.019 over the cases above 5 ms (Proven at or under 1.05), 1m50s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/bytebuf, bench/perf/bytespredicate, bench/perf/bytessearch, bench/perf/bytessplit, bench/perf/bytestransform, bench/perf/gomath, bench/perf/mapiter, bench/perf/repeat, bench/perf/sort, bench/perf/strconv, bench/perf/strsplit, bench/perf/trimchars, bench/perf/unicode; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go | Go package math ms (decision 0721) |
|---|---|---|---|---|
| bench/perf/bytebuf/none (under 5 ms: startup) | 1.4 | 1.2 | 0.848 | |
| bench/perf/bytebuf/small_concat | 13.1 | 10.5 | 0.803 | |
| bench/perf/bytebuf/small_views | 12.5 | 10.0 | 0.802 | |
| bench/perf/bytebuf/large1_concat (under 5 ms: startup) | 3.8 | 2.0 | 0.525 | |
| bench/perf/bytebuf/large1_views (under 5 ms: startup) | 3.8 | 1.6 | 0.427 | |
| bench/perf/bytebuf/small_buf | 12.8 | 10.2 | 0.797 | |
| bench/perf/bytebuf/small_vrec | 12.8 | 10.1 | 0.788 | |
| bench/perf/bytebuf/large1_buf (under 5 ms: startup) | 3.8 | 1.6 | 0.415 | |
| bench/perf/bytebuf/large64_buf | 122.4 | 28.9 | 0.236 | |
| bench/perf/bytebuf/large64_views | 115.4 | 27.2 | 0.236 | |
| bench/perf/bytespredicate/index (under 5 ms: startup) | 1.7 | 1.3 | 0.797 | |
| bench/perf/bytespredicate/find_where (under 5 ms: startup) | 4.1 | 2.0 | 0.498 | |
| bench/perf/bytespredicate/rfind_where (under 5 ms: startup) | 3.2 | 1.9 | 0.591 | |
| bench/perf/bytespredicate/contains_where | 9.0 | 4.4 | 0.496 | |
| bench/perf/bytespredicate/trim_where (under 5 ms: startup) | 4.6 | 2.3 | 0.495 | |
| bench/perf/bytespredicate/trim_start_where (under 5 ms: startup) | 3.5 | 2.5 | 0.711 | |
| bench/perf/bytespredicate/trim_end_where (under 5 ms: startup) | 2.1 | 1.5 | 0.719 | |
| bench/perf/bytespredicate/split_where | 24.2 | 13.6 | 0.563 | |
| bench/perf/bytespredicate/map_runes | 17.6 | 14.9 | 0.848 | |
| bench/perf/bytessearch/index (under 5 ms: startup) | 1.7 | 1.3 | 0.774 | |
| bench/perf/bytessearch/find_any | 6.3 | 5.1 | 0.820 | |
| bench/perf/bytessearch/rfind_any | 7.0 | 6.0 | 0.847 | |
| bench/perf/bytessearch/contains_any | 6.3 | 4.8 | 0.768 | |
| bench/perf/bytessearch/contains_rune (under 5 ms: startup) | 2.7 | 3.7 | 1.363 | |
| bench/perf/bytessearch/cut (under 5 ms: startup) | 2.4 | 2.7 | 1.104 | |
| bench/perf/bytessearch/cut_prefix (under 5 ms: startup) | 2.0 | 1.6 | 0.797 | |
| bench/perf/bytessearch/cut_suffix (under 5 ms: startup) | 2.0 | 1.6 | 0.804 | |
| bench/perf/bytessearch/split_after | 15.6 | 10.3 | 0.661 | |
| bench/perf/bytessplit/index (under 5 ms: startup) | 2.2 | 1.5 | 0.701 | |
| bench/perf/bytessplit/split | 20.2 | 11.7 | 0.579 | |
| bench/perf/bytessplit/split_n | 9.7 | 4.9 | 0.505 | |
| bench/perf/bytessplit/fields | 21.7 | 11.6 | 0.535 | |
| bench/perf/bytessplit/trim (under 5 ms: startup) | 3.1 | 3.0 | 0.972 | |
| bench/perf/bytessplit/affix (under 5 ms: startup) | 2.9 | 2.9 | 1.010 | |
| bench/perf/bytestransform/index (under 5 ms: startup) | 2.0 | 1.4 | 0.713 | |
| bench/perf/bytestransform/replace | 14.0 | 11.3 | 0.806 | |
| bench/perf/bytestransform/replace_n | 9.4 | 6.0 | 0.639 | |
| bench/perf/bytestransform/lower | 17.0 | 9.4 | 0.556 | |
| bench/perf/bytestransform/upper | 17.7 | 11.9 | 0.673 | |
| bench/perf/bytestransform/fold (under 5 ms: startup) | 3.1 | 2.6 | 0.844 | |
| bench/perf/bytestransform/repeat | 7.1 | 2.6 | 0.367 | |
| bench/perf/bytestransform/lossy | 5.2 | 4.7 | 0.914 | |
| bench/perf/bytestransform/valid | 9.4 | 4.6 | 0.492 | |
| bench/perf/gomath/none (under 5 ms: startup) | 1.4 | 1.2 | 0.828 | |
| bench/perf/gomath/erf | 47.8 | 46.4 | 0.969 | 36.0 |
| bench/perf/gomath/erfc | 55.3 | 54.2 | 0.980 | 40.5 |
| bench/perf/gomath/erfinv | 17.4 | 17.7 | 1.019 | |
| bench/perf/gomath/erfcinv | 18.2 | 17.3 | 0.950 | |
| bench/perf/gomath/gamma | 83.3 | 79.1 | 0.950 | |
| bench/perf/gomath/lgamma | 26.9 | 26.7 | 0.992 | |
| bench/perf/gomath/next_after | 6.6 | 6.4 | 0.978 | |
| bench/perf/gomath/remainder | 151.0 | 103.6 | 0.686 | |
| bench/perf/gomath/acosh | 24.5 | 21.0 | 0.857 | |
| bench/perf/gomath/exp | 21.0 | 16.1 | 0.766 | 14.1 |
| bench/perf/gomath/pow | 79.2 | 66.5 | 0.840 | 69.4 |
| bench/perf/mapiter/none (under 5 ms: startup) | 1.5 | 1.2 | 0.809 | |
| bench/perf/mapiter/walk | 22.0 | 9.9 | 0.449 | |
| bench/perf/mapiter/keys | 19.3 | 3.5 | 0.183 | |
| bench/perf/repeat/none (under 5 ms: startup) | 1.4 | 1.1 | 0.832 | |
| bench/perf/repeat/byte1 | 339.6 | 275.0 | 0.810 | |
| bench/perf/repeat/byte3 | 339.4 | 242.7 | 0.715 | |
| bench/perf/repeat/byte100 | 266.5 | 255.2 | 0.958 | |
| bench/perf/sort/ints | 69.3 | 62.4 | 0.900 | |
| bench/perf/sort/strs | 35.8 | 29.0 | 0.808 | |
| bench/perf/sort/floats | 80.2 | 80.1 | 0.999 | |
| bench/perf/sort/by | 170.1 | 108.3 | 0.637 | |
| bench/perf/strconv/atoi | 34.0 | 34.3 | 1.009 | |
| bench/perf/strconv/itoa | 105.7 | 63.9 | 0.605 | |
| bench/perf/strconv/atof | 127.4 | 83.3 | 0.653 | |
| bench/perf/strconv/ftoa | 290.4 | 213.6 | 0.736 | |
| bench/perf/strsplit/split | 14.0 | 8.4 | 0.600 | |
| bench/perf/strsplit/split_one | 10.5 | 5.9 | 0.563 | |
| bench/perf/strsplit/split_multi | 8.7 | 7.8 | 0.900 | |
| bench/perf/strsplit/split_absent | 6.7 | 4.9 | 0.724 | |
| bench/perf/strsplit/split_empty | 28.2 | 17.2 | 0.611 | |
| bench/perf/strsplit/split_after | 10.4 | 7.2 | 0.686 | |
| bench/perf/strsplit/lines | 9.8 | 6.8 | 0.695 | |
| bench/perf/strsplit/bytes_split | 15.6 | 7.4 | 0.477 | |
| bench/perf/strsplit/bytes_fields | 21.1 | 9.7 | 0.460 | |
| bench/perf/strsplit/fields | 15.6 | 9.2 | 0.593 | |
| bench/perf/strsplit/index (under 5 ms: startup) | 2.0 | 1.5 | 0.756 | |
| bench/perf/trimchars/index | 13.8 | 6.1 | 0.446 | |
| bench/perf/trimchars/trim | 21.9 | 21.2 | 0.967 | |
| bench/perf/trimchars/trim_start | 20.0 | 17.6 | 0.877 | |
| bench/perf/trimchars/trim_end | 18.8 | 17.4 | 0.929 | |
| bench/perf/trimchars/one | 10.7 | 10.6 | 0.991 | |
| bench/perf/trimchars/pair | 35.3 | 26.6 | 0.754 | |
| bench/perf/trimchars/abc | 36.5 | 31.4 | 0.859 | |
| bench/perf/trimchars/uni | 38.6 | 36.5 | 0.946 | |
| bench/perf/unicode/lower | 106.2 | 12.1 | 0.114 | |
| bench/perf/unicode/upper | 115.4 | 12.1 | 0.105 | |
| bench/perf/unicode/lower_u | 243.7 | 141.1 | 0.579 | |
| bench/perf/unicode/upper_u | 245.5 | 151.3 | 0.616 | |
| bench/perf/unicode/lower_az | 25.1 | 5.2 | 0.208 | |
| bench/perf/unicode/upper_az | 27.7 | 5.4 | 0.195 | |
| bench/perf/unicode/eqlower | 706.4 | 292.9 | 0.415 | |
| bench/perf/unicode/classify | 19.2 | 6.7 | 0.350 | |
| bench/perf/unicode/runecase | 13.2 | 10.9 | 0.827 | |
| bench/perf/unicode/classes | 47.3 | 21.3 | 0.449 | |
| bench/perf/unicode/latin1 (under 5 ms: startup) | 3.6 | 3.3 | 0.917 | |
| bench/perf/unicode/title | 9.6 | 9.4 | 0.972 | |
| bench/perf/unicode/utf8len (under 5 ms: startup) | 4.5 | 4.0 | 0.882 | |
| bench/perf/unicode/quote | 15.2 | 5.5 | 0.360 | |
| bench/perf/unicode/indexfunc | 349.3 | 291.0 | 0.833 | |
| bench/perf/unicode/trimfunc | 6.9 | 3.9 | 0.561 | |
| bench/perf/unicode/fieldsfunc | 30.7 | 10.6 | 0.346 | |
| bench/perf/unicode/mapfunc | 204.8 | 117.9 | 0.576 | |
