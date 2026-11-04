# std twin rows: url and checksum (2026-11-04, linux/arm64 host 1, n=6)

New twins bench/perf/url (Go's net/url) and bench/perf/checksum (hash/crc32, crc64, fnv, adler32). Ratio is Xo wall over Go wall.

| workload | before | after |
|---|---|---|
| url escape | 1.552 | about 1.08 (A/B 0.695) |
| url unescape | 0.883 | unchanged |
| url query | 1.497 | about 1.18 (A/B 0.788) |
| url encode | 1.207 | about 1.08 (A/B 0.894) |
| crc32 | 0.886 | |
| crc64 | 1.001 | |
| fnv32a | 0.982 | |
| fnv64a | 0.987 | |
| adler32 | 0.942 | |

The "after" figures are the before ratio times the A/B ratio, not a fresh twin run.
