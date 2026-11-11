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

## Metrics (2026-11-11): derive is slower, so metrics export keeps the writer

bench/perf/otlp `metrics` encodes a registry of a counter, a gauge, a
histogram (latency buckets) and a summary, 50 labelled series each,
4,000 times. B wrote the metrics messages (Metric with its data a oneof
enum, NumberDataPoint with as_double a oneof, Histogram and Summary
points) as derive Proto structs; its bytes equal the writer's on the
corpus and on a registry with summaries, function families, a zero gauge
and an empty bucket list (SHA-256, arm64 and llvm). linux/arm64 host 1,
`--ab --rounds=10`, A main 2d26a633c's std:

| workload | A wall | B wall | B / A (95%) |
|---|---|---|---|
| metrics | 205.4 ms | 247.4 ms | 1.205 (1.198 to 1.211), worse |
| traces (unchanged code, control) | 454.2 ms | 449.2 ms | 0.989 (0.984 to 0.994) |

`XO_PROF=cpu`: proto_marshal__MetricMsg is 67% of B, its time in
put_fixed64, put_varint and appends. Like the writer, the generated
marshal makes a Bytes for each nested message (`put_bytes(proto_marshal__X(v))`),
and B adds building the structs: a KeyValueMsg and an AnyValueMsg per
label per point, a List per point. Traces gained because its writer
copied more (a Bytes per KeyValue, AnyValue, Event and Span copied into
each parent).

Next experiment: the generated marshal writes a nested message into its
parent's builder with a reserved length prefix (protobuf-go computes
sizes first; a back-patched one byte prefix moved up when longer is the
cheaper form), so no message is a Bytes of its own; then this row again.

