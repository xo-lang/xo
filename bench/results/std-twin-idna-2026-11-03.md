# std twin idna (and norm): host rows (2026-11-03)

Host 1 (linux/arm64, `twin-ab2`, n=15, ratio is Xo wall over Go wall), as rows16. idna ab4 is 623895699 (validate_and_map and validate_label decode in place); idna ab5 and norm ab6 add std/norm's normalize_run decoding in place.

| module | workload | rows16 | ab4 | ab5 / ab6 |
|---|---|---|---|---|
| idna | ascii | 0.72 | 0.71 | 0.74 |
| idna | mixed | 1.69 | 1.57 | 1.54 |
| idna | long | 1.65 | 1.41 | 1.49 |
| norm | nfc | 1.64 | | 1.55 |
| norm | nfd | 1.13 | | 1.08 |
| norm | nfkc | 1.11 | | 1.08 |

Not Proven (ADR 0461: at most about 1.05); idna mixed and long and norm NFC remain slower than Go. Run to run spread on long is about 0.08. Remaining cost per the Mac profile: allocation of the per run decomposition lists, `compose`, and `lookup` (13 percent of idna mixed).

## Per phase (host 1, n=15; ratio of Xo to Go wall)

Workloads in bench/perf/idna: `map` ToUnicode of 8 composed non-ASCII names (map, validate, NFC check), `nfcmap` of 8 decomposed names (map and compose), `enc` ToASCII of the map names (adds Punycode encode), `dec` ToUnicode of their Punycode forms (Punycode decode and validate); 4000 rounds of 8 names each.

| workload | Go ms | Xo ms before (ab6) | ratio | Xo ms after (ab7) | ratio |
|---|---|---|---|---|---|
| map | 4.30 | 6.17 | 1.43 | 6.19 | 1.42 |
| nfcmap | 15.0 | 9.78 | 0.65 | 9.94 | 0.67 |
| enc | 9.85 | 15.84 | 1.61 | 11.78 | 1.20 |
| dec | 14.4 | 23.05 | 1.61 | 18.51 | 1.29 |
| mixed | 21.6 | 33.4 | 1.55 | 28.83 | 1.34 |
| long | 5.54 | 8.48 | 1.53 | 6.16 | 1.10 |

"After" is 45cac086e: Punycode encode and decode write into a BytesBuilder instead of allocating a string per rune (interpolation). Reading the table: composing (nfcmap) is already faster than Go's; Punycode went from 1.6 to 1.2 and 1.3; what is left is `map` at 1.42 (the lookup and validation of already composed names, before quickSpan reached NFC) and the Punycode loops. All still above ADR 0461's 1.05 except long (1.10).
