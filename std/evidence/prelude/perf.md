kind: perf
module: prelude
result: pass
commit: f095943e6
source: d6e964db07930890a143a079c4f47d91baaed2e7200c989529b06737e272921a
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" prelude
measure: 95 cases of 12 twin directories, worst Xo / Go 1.004 over the cases above 5 ms (Proven at or under 1.05), 1m41s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/bytebuf, bench/perf/bytespredicate, bench/perf/bytessearch, bench/perf/bytessplit, bench/perf/bytestransform, bench/perf/mapiter, bench/perf/repeat, bench/perf/sort, bench/perf/strconv, bench/perf/strsplit, bench/perf/trimchars, bench/perf/unicode; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/bytebuf/none (under 5 ms: startup) | 1.5 | 1.2 | 0.803 |
| bench/perf/bytebuf/small_concat | 13.1 | 10.5 | 0.802 |
| bench/perf/bytebuf/small_views | 13.0 | 9.9 | 0.758 |
| bench/perf/bytebuf/large1_concat (under 5 ms: startup) | 4.5 | 2.1 | 0.459 |
| bench/perf/bytebuf/large1_views (under 5 ms: startup) | 4.4 | 1.6 | 0.371 |
| bench/perf/bytebuf/small_buf | 13.3 | 10.4 | 0.780 |
| bench/perf/bytebuf/small_vrec | 13.8 | 10.3 | 0.744 |
| bench/perf/bytebuf/large1_buf (under 5 ms: startup) | 4.2 | 1.6 | 0.386 |
| bench/perf/bytebuf/large64_buf | 122.2 | 28.5 | 0.233 |
| bench/perf/bytebuf/large64_views | 122.9 | 27.9 | 0.227 |
| bench/perf/bytespredicate/index (under 5 ms: startup) | 1.8 | 1.3 | 0.737 |
| bench/perf/bytespredicate/find_where (under 5 ms: startup) | 4.3 | 2.0 | 0.471 |
| bench/perf/bytespredicate/rfind_where (under 5 ms: startup) | 3.7 | 2.0 | 0.541 |
| bench/perf/bytespredicate/contains_where | 9.3 | 4.8 | 0.514 |
| bench/perf/bytespredicate/trim_where (under 5 ms: startup) | 4.8 | 2.4 | 0.502 |
| bench/perf/bytespredicate/trim_start_where (under 5 ms: startup) | 3.6 | 2.4 | 0.675 |
| bench/perf/bytespredicate/trim_end_where (under 5 ms: startup) | 2.3 | 1.5 | 0.638 |
| bench/perf/bytespredicate/split_where | 25.2 | 13.9 | 0.550 |
| bench/perf/bytespredicate/map_runes | 18.5 | 16.3 | 0.880 |
| bench/perf/bytessearch/index (under 5 ms: startup) | 1.8 | 1.3 | 0.755 |
| bench/perf/bytessearch/find_any | 6.5 | 5.1 | 0.785 |
| bench/perf/bytessearch/rfind_any | 7.1 | 5.8 | 0.819 |
| bench/perf/bytessearch/contains_any | 6.5 | 5.1 | 0.780 |
| bench/perf/bytessearch/contains_rune (under 5 ms: startup) | 3.2 | 3.5 | 1.086 |
| bench/perf/bytessearch/cut (under 5 ms: startup) | 3.1 | 3.0 | 0.949 |
| bench/perf/bytessearch/cut_prefix (under 5 ms: startup) | 2.3 | 1.7 | 0.717 |
| bench/perf/bytessearch/cut_suffix (under 5 ms: startup) | 2.2 | 1.6 | 0.762 |
| bench/perf/bytessearch/split_after | 18.8 | 9.7 | 0.515 |
| bench/perf/bytessplit/index (under 5 ms: startup) | 2.2 | 1.5 | 0.670 |
| bench/perf/bytessplit/split | 22.6 | 12.1 | 0.534 |
| bench/perf/bytessplit/split_n | 10.4 | 5.3 | 0.508 |
| bench/perf/bytessplit/fields | 23.0 | 12.4 | 0.537 |
| bench/perf/bytessplit/trim (under 5 ms: startup) | 3.2 | 2.5 | 0.787 |
| bench/perf/bytessplit/affix (under 5 ms: startup) | 2.8 | 2.8 | 1.016 |
| bench/perf/bytestransform/index (under 5 ms: startup) | 2.2 | 1.6 | 0.697 |
| bench/perf/bytestransform/replace | 14.8 | 11.8 | 0.801 |
| bench/perf/bytestransform/replace_n | 10.8 | 6.0 | 0.554 |
| bench/perf/bytestransform/lower | 17.8 | 12.0 | 0.671 |
| bench/perf/bytestransform/upper | 18.1 | 11.5 | 0.633 |
| bench/perf/bytestransform/fold (under 5 ms: startup) | 2.9 | 2.6 | 0.903 |
| bench/perf/bytestransform/repeat | 7.9 | 2.8 | 0.351 |
| bench/perf/bytestransform/lossy | 5.3 | 4.3 | 0.819 |
| bench/perf/bytestransform/valid | 9.9 | 4.5 | 0.454 |
| bench/perf/mapiter/none (under 5 ms: startup) | 1.6 | 1.2 | 0.752 |
| bench/perf/mapiter/walk | 21.9 | 9.8 | 0.447 |
| bench/perf/mapiter/keys | 20.1 | 3.6 | 0.178 |
| bench/perf/repeat/none (under 5 ms: startup) | 1.5 | 1.1 | 0.744 |
| bench/perf/repeat/byte1 | 394.7 | 395.5 | 1.002 |
| bench/perf/repeat/byte3 | 355.4 | 352.9 | 0.993 |
| bench/perf/repeat/byte100 | 370.4 | 371.2 | 1.002 |
| bench/perf/sort/ints | 70.3 | 63.7 | 0.905 |
| bench/perf/sort/strs | 36.6 | 29.5 | 0.806 |
| bench/perf/sort/floats | 80.9 | 81.3 | 1.004 |
| bench/perf/sort/by | 172.2 | 111.4 | 0.647 |
| bench/perf/strconv/atoi | 34.0 | 31.2 | 0.916 |
| bench/perf/strconv/itoa | 108.9 | 64.8 | 0.595 |
| bench/perf/strconv/atof | 129.8 | 94.6 | 0.728 |
| bench/perf/strconv/ftoa | 294.3 | 210.9 | 0.717 |
| bench/perf/strsplit/split | 16.5 | 8.8 | 0.529 |
| bench/perf/strsplit/split_one | 11.9 | 5.9 | 0.496 |
| bench/perf/strsplit/split_multi | 9.6 | 7.9 | 0.821 |
| bench/perf/strsplit/split_absent | 7.8 | 4.0 | 0.512 |
| bench/perf/strsplit/split_empty | 34.6 | 17.8 | 0.515 |
| bench/perf/strsplit/split_after | 12.4 | 7.5 | 0.605 |
| bench/perf/strsplit/lines | 10.3 | 9.5 | 0.922 |
| bench/perf/strsplit/bytes_split | 17.4 | 6.6 | 0.381 |
| bench/perf/strsplit/bytes_fields | 21.3 | 11.5 | 0.539 |
| bench/perf/strsplit/fields | 16.5 | 9.4 | 0.570 |
| bench/perf/strsplit/index (under 5 ms: startup) | 2.0 | 1.4 | 0.696 |
| bench/perf/trimchars/index | 13.9 | 6.2 | 0.449 |
| bench/perf/trimchars/trim | 21.5 | 21.1 | 0.982 |
| bench/perf/trimchars/trim_start | 19.8 | 17.6 | 0.885 |
| bench/perf/trimchars/trim_end | 18.9 | 17.5 | 0.927 |
| bench/perf/trimchars/one | 10.6 | 10.4 | 0.977 |
| bench/perf/trimchars/pair | 35.3 | 26.1 | 0.737 |
| bench/perf/trimchars/abc | 36.8 | 30.8 | 0.838 |
| bench/perf/trimchars/uni | 39.6 | 36.6 | 0.924 |
| bench/perf/unicode/lower | 110.6 | 12.4 | 0.112 |
| bench/perf/unicode/upper | 123.8 | 12.7 | 0.102 |
| bench/perf/unicode/lower_u | 247.4 | 141.9 | 0.574 |
| bench/perf/unicode/upper_u | 249.9 | 151.2 | 0.605 |
| bench/perf/unicode/lower_az | 25.9 | 5.0 | 0.194 |
| bench/perf/unicode/upper_az | 29.7 | 4.9 | 0.166 |
| bench/perf/unicode/eqlower | 719.3 | 294.3 | 0.409 |
| bench/perf/unicode/classify | 19.1 | 7.0 | 0.364 |
| bench/perf/unicode/runecase | 13.1 | 10.3 | 0.793 |
| bench/perf/unicode/classes | 48.1 | 21.3 | 0.444 |
| bench/perf/unicode/latin1 (under 5 ms: startup) | 3.9 | 3.3 | 0.846 |
| bench/perf/unicode/title | 10.1 | 9.1 | 0.896 |
| bench/perf/unicode/utf8len (under 5 ms: startup) | 4.5 | 3.6 | 0.798 |
| bench/perf/unicode/quote | 15.3 | 5.6 | 0.368 |
| bench/perf/unicode/indexfunc | 348.9 | 291.4 | 0.835 |
| bench/perf/unicode/trimfunc | 7.3 | 4.0 | 0.541 |
| bench/perf/unicode/fieldsfunc | 34.0 | 11.8 | 0.347 |
| bench/perf/unicode/mapfunc | 210.4 | 117.5 | 0.559 |
