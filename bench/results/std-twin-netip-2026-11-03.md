# std twin netip: host rows (2026-11-03)

Host 1, `xo bench --ab`, n=6 (the program parses two addresses, `is_private`, and displays both, 200,000 times). Ratio is Xo wall over Go wall.

| run | Xo at | Go ms | Xo ms | ratio | interval |
|---|---|---|---|---|---|
| n1 | 781d566e4 | 19.04 | 35.82 | 1.88 | 1.85-1.93 |
| n2 | bce92494e (string4/string6 without a UTF-8 check, byte scan for %) | 19.7 | 35.3 | 1.79 | 1.76-1.83 |

The remaining cost is the representation (a heap `Bytes` per address and byte-at-a-time construction): decision 0485 (Accepted 2026-11-03).

## After decision 0485's representation (host 1, `--early`, n=6, Xo at 060252aac, per workload)

| workload | Go ms | Xo ms | ratio | interval |
|---|---|---|---|---|
| mixed | 18.88 | 24.79 | 1.313 | 1.282-1.34 |
| parse4 | 5.263 | 7.249 | 1.377 | 1.337-1.426 |
| parse6 | 10.44 | 17.78 | 1.702 | 1.663-1.752 |
| display4 | 6.783 | 13.43 | 1.98 | 1.913-2.076 |
| display6 | 15.2 | 10.17 | 0.669 | 0.656-0.681 |
| compare | 16.1 | 32.58 | 2.023 | 1.907-2.079 |

The mixed row went from 1.79 to 1.31. Host 2 A/B of the representation change: mixed 0.766 of before. Open: display4 and compare (2.0), parse6 (1.7), parse4 (1.38).

## 2026-11-04 (host 1 A/B, n=6 unless noted)

| change | workload | ratio of before | interval |
|---|---|---|---|
| string4 packed in two words | display4 | 0.637 | 0.629-0.645 |
| hex digits from a table | parse6 | 0.901 | 0.884-0.925 |
| ipv4_fields in place (reverted) | parse4 | 1.03 | 1.006-1.042 (n=15) |

Against Go (rows above): display4 about 1.26, parse6 about 1.50. Left: compare 2.02 and part of display4 are RC drops of the zone Str (Lang 1: inline drop test), parse4 1.38.

