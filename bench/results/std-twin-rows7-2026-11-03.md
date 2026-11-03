# Timing rows against Go, part 7 (2026-11-03)

Host 1, `xo bench --ab --rounds=15`, bench/perf/cbor (a 20 KB document of 150 records with ints, floats, text, byte strings, lists and nested maps; 100 passes; `none` is the setup) at the change set that adds std/cbor (decision 0468). Go has no CBOR in its standard library: the twin is github.com/fxamacker/cbor/v2 v2.9.0 (vendored in bench/perf/cbor/govendor), decoding into `any` and marshalling that tree (`canon` with CoreDetEncOptions).

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| cbor | decode | 20.8 | 8.829 | 0.425 | 0.418 to 0.431 | 0.606 |
| cbor | encode | 15.01 | 3.962 | 0.264 | 0.258 to 0.27 | 0.791 |
| cbor | canon | 19.34 | 12.0 | 0.621 | 0.603 to 0.631 | 0.628 |
| cbor | none | 0.853 | 0.504 | 0.591 | 0.552 to 0.654 | 1.0 |
