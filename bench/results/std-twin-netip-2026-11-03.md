# std twin netip: host rows (2026-11-03)

Host 1, `xo bench --ab`, n=6 (the program parses two addresses, `is_private`, and displays both, 200,000 times). Ratio is Xo wall over Go wall.

| run | Xo at | Go ms | Xo ms | ratio | interval |
|---|---|---|---|---|---|
| n1 | 781d566e4 | 19.04 | 35.82 | 1.88 | 1.85-1.93 |
| n2 | bce92494e (string4/string6 without a UTF-8 check, byte scan for %) | 19.7 | 35.3 | 1.79 | 1.76-1.83 |

The remaining cost is the representation (a heap `Bytes` per address and byte-at-a-time construction): decision 0485 (Accepted 2026-11-03).
