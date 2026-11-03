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
