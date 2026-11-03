# Timing rows against Go, part 8 (2026-11-03)

Host 1, `xo bench --ab --rounds=15`, bench/perf/xml (a 1.7 MB document of 20,000 items; 8 passes; `none` is the document build) at the change set that adds `xml.Reader` (decision 0463). `stream` is the `tokens` loop through a Reader (one token alive at a time); its Go twin runs the same Decoder.Token loop as `tokens`. The memory column is the harness's peak memory ratio, Xo to Go.

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| xml | none | 3.206 | 2.984 | 0.931 | 0.869 to 0.957 | 0.939 |
| xml | tokens | 217.9 | 254.4 | 1.167 | 1.161 to 1.183 | 7.215 |
| xml | stream | 218.6 | 258.9 | 1.184 | 1.183 to 1.188 | 0.701 |
| xml | tree | 305.1 | 299.7 | 0.982 | 0.977 to 0.996 | 3.917 |

A program that walks the tokens through a Reader runs in 0.70 of Go's peak memory against 7.2 for the list of `tokens`, at the same wall time (1.18 against 1.17). The memory column of `tokens` and `tree` is measured the same way as in rows (the earlier 3.0 and 2.1 came from a different harness run; the ratio here includes the harness's baseline).
