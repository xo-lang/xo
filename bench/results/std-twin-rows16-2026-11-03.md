# std twin rows 16: norm and idna after the byte tables and block indexes (2026-11-03)

Host 1 (linux/arm64, `twin-ab2`, n=15, ratio is Xo wall over Go wall, Go is x/text v0.43.0 and x/net v0.60.0; Unicode 15.0.0), Xo at 3e47b249a. Raw rows: norm-ab5 and idna-ab3 in the Lang 3 scratchpad (norm-ab4 is a repeat).

| module | workload | Go ms | Xo ms | ratio | interval | verdict |
|---|---|---|---|---|---|---|
| norm | none | 4.07 | 6.95 | 1.71 | 1.66-1.76 | worse (startup) |
| norm | ascii | 7.37 | 8.99 | 1.22 | 1.19-1.26 | worse |
| norm | nfc | 89.7 | 147.0 | 1.64 | 1.62-1.65 | worse |
| norm | nfd | 122.4 | 138.2 | 1.13 | 1.11-1.14 | worse |
| norm | nfkc | 132.6 | 147.6 | 1.11 | 1.11-1.13 | worse |
| idna | none | 0.544 | 0.287 | 0.53 | 0.46-0.58 | better |
| idna | ascii | 26.88 | 19.33 | 0.72 | 0.71-0.73 | better |
| idna | mixed | 21.10 | 35.72 | 1.69 | 1.67-1.72 | worse |
| idna | long | 5.404 | 8.93 | 1.65 | 1.57-1.71 | worse |

Against the earlier host rows: norm NFC 2.14 to 1.64 (byte records, block indexes, one class list per run), NFD 1.28 to 1.13, NFKC 1.45 to 1.11; idna mixed 2.29 to 1.69 and long 1.98 to 1.65 (the 7 byte ranges with a block index). idna ascii is faster than Go's.

Not Proven (ADR 0461: at most about 1.05). Where the time goes (Mac profile of NFC): `normalize` 24 percent, `compose` 14, `normalize_run` 14, `push_decomp` 13, allocation of the per run lists about 15. Next: decompose into one reused list per call, not per run; idna's per label allocations in `process`.
