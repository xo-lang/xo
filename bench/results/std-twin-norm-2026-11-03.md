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

## After rt.cast (f0b7c8596; host 1, n=15)

| workload | Go ms | Xo ms | ratio | interval | before (ab7) |
|---|---|---|---|---|---|
| none | 4.113 | 6.815 | 1.66 | 1.59-1.72 | 1.64 |
| ascii | 7.346 | 9.18 | 1.25 | 1.21-1.27 | 1.25 |
| nfc | 90.19 | 66.45 | 0.74 | 0.73-0.75 | 0.88 |
| nfd | 122.3 | 113.1 | 0.925 | 0.90-0.94 | 1.09 |
| nfkc | 127.5 | 89.57 | 0.70 | 0.68-0.71 | 0.77 |

The output Str is built with `rt.cast[Bytes, Str]` (valid UTF-8 by construction), without the validation pass of `to_str`; the Go backend lowers it to `rt.Rt_cast` (std/rt/str.go, a free conversion since Bytes is an immutable string there). NFC, NFD and NFKC are all faster than x/text; `none` and `ascii` are the startup and ASCII scan (about 2.7 ms of fixed setup). Proven per ADR 0461 (at most about 1.05) on nfc, nfd and nfkc.
