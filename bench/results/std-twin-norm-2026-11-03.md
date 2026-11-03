# std twin norm: host rows (2026-11-03)

Host 1 (linux/arm64, `twin-ab2`, n=15; ratio is Xo wall over Go wall; Go is x/text v0.43.0, Unicode 15.0.0).

| workload | after byte tables (rows16) | in-place decode (ab6) | after quickSpan (ab7, 9dbd15044) | Go ms | Xo ms | interval |
|---|---|---|---|---|---|---|
| none | 1.71 | 1.51 | 1.64 | 4.18 | 6.88 | 1.59-1.75 |
| ascii | 1.22 | 1.45 | 1.25 | 7.51 | 9.39 | 1.22-1.30 |
| nfc | 1.64 | 1.55 | 0.88 | 91.78 | 80.9 | 0.87-0.89 |
| nfd | 1.13 | 1.08 | 1.09 | 119.1 | 130.1 | 1.06-1.11 |
| nfkc | 1.11 | 1.08 | 0.77 | 129.6 | 100.3 | 0.76-0.78 |

NFC and NFKC are faster than Go since the quickSpan table: runes from U+C0 that NFC (NFKC) cannot change, reorder or combine backward are copied without a decomposition, as Go's quickSpan does. NFD (no composition, so no quickSpan) is 1.09, at the line of ADR 0461's about 1.05 but above it; `none` and `ascii` are startup and the ASCII scan (the Xo binary starts later than Go's, 6.9 against 4.2 ms of setup, a fixed cost of about 2.7 ms that the larger workloads amortize).

Size, same method as std-idna-size-2026-11-03.md, at 9dbd15044 (darwin/arm64): importing std/norm adds 82,848 bytes to a program (Go +100,096 stripped, +120,672 default); code +12.3 KB against Go's +23.5 KB; table data 64.0 KB against Go's 64.5 KB (the quickSpan tables are 2.9 KB of it).
