# std/otlp traces: derive Proto structs against the hand writer (2026-11-11)

Decision 0549 item 3: std/otlp may write its messages as derive Proto
structs only if its timing does not regress. bench/perf/otlp `traces`
encodes 200 spans (OpenTelemetry ids, a parent, kind, four attributes,
two events, an error status every ninth, two scopes) 4,000 times.

linux/arm64 offload host 1, `xo bench --ab --rounds=10`, both built by
one xo (main 3a1a5dc07's compiler) for linux/arm64 --release: A with
main's std (the builder writer, each nested message a Bytes of its own),
B with the trace messages as derive Proto structs marshaled once.

| metric | A | B | B / A (95%) |
|---|---|---|---|
| wall_ms | 472.6 | 410.9 | 0.869 (0.863 to 0.877), better |
| max_rss_kb | 1110 | 1110 | 1.0, within noise |

Both print `traces 912332731` (the same bytes). The generated marshal
sizes each message once and writes into one buffer, where the hand
writer froze a Bytes per KeyValue, AnyValue, Event and Span and copied
it into its parent.
