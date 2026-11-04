# std twin html: escape and unescape host rows (2026-11-03)

Host 1, `xo bench --ab --early`, n=6, Xo at 781d566e4. Ratio is Xo wall over Go wall.

| workload | Go ms | Xo ms | ratio | interval | before (rows2) |
|---|---|---|---|---|---|
| escape | 122.5 | 168.3 | 1.37 | 1.34-1.40 | 1.75 |
| unescape | 289.3 | 156.2 | 0.54 | 0.53-0.56 | 0.65 |

`escape_string` writes each of its five replacements with `write_byte`: the Mac profile showed the `write_str` of a literal (`rs_append`, `move_at`, `rs_concat`) at 29 percent beside the scan loop. A second change (testing each byte with the 256 byte table in the copy loop instead of five comparisons) measured 193 to 202 ms on the Mac, no gain, and was dropped. escape is not Proven (ADR 0461); the remaining cost is the two passes over the text and the per-run `write` of a slice.
