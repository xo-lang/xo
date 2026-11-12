# std/otlp changes

## 2026-11-11: perf: metrics export encodes derive Proto message structs

- Kind: perf
- Commit: this push
- Found by: decision 0549 item 3, after derive Proto's size pass
- Go reference: go.opentelemetry.io/proto/otlp (metrics.proto), as protobuf-go marshals it
- Timing: bench/results/otlp-derive-2026-11-11.md third table: bench/perf/otlp metrics 0.855 of the hand writer's wall time (linux/arm64 host 1, --ab)
- Test: the otlp corpus on go, arm64, llvm; std/native/otlp_test.xo; a registry with summaries, function families, a zero gauge and an empty bucket list hashes the same as the writer's output
- Shipped in: not yet released

## 2026-11-11: perf: trace export encodes derive Proto message structs

- Kind: perf
- Commit: this push
- Found by: decision 0549 item 3 (derive in std; otlp may use message structs if its timing does not regress)
- Go reference: go.opentelemetry.io/proto/otlp (trace.proto, common.proto, resource.proto), as protobuf-go marshals it
- Timing: bench/results/otlp-derive-2026-11-11.md: bench/perf/otlp traces 0.869 of the hand writer's wall time (linux/arm64 host 1, --ab)
- Test: the otlp corpus (protobuf-go's bytes) on go, arm64, llvm; std/native/otlp_test.xo (the OpenTelemetry Go oracle) on arm64, llvm
- Shipped in: not yet released

`encode_traces` builds the request as derive Proto structs
(ExportTraceServiceRequest down to AnyValue, whose value is a oneof)
and marshals them once, where it wrote each nested message to a Bytes
of its own and copied it into its parent. Output unchanged.

## 2026-11-09: feature: encode_traces writes the OpenTelemetry span fields (decision 0580)

- Kind: feature
- Commit: this push
- Found by: Xo Azure 1 (tracing/azotel)
- Go reference: opentelemetry-proto trace.proto (Span 1 to 15, Event, Status, KeyValue, AnyValue) and the OpenTelemetry Go exporter
- Test: std/native/otlp_test.xo (hand vectors) and the OpenTelemetry Go oracle (otel v1.47.0 through otlptracehttp; Xo Azure 1): std/native/trace_test.xo's generated tests (scripts/oteloracle/gen.py) compare the request bodies of 7 cases (root and child, every kind, every attribute type, attribute overwrite, events, ten status sequences, scopes with versions and schema URLs) byte for byte
- Shipped in: not yet released

`encode_traces` writes a span's own trace, span and parent ids, kind, attributes, events, status, nanosecond times, and groups spans by (scope, version, schema URL) into ScopeSpans (a span with no scope uses the Resource's); a span that leaves them at their defaults encodes as before. A span with its own span id is written as the OpenTelemetry Go SDK writes it: Status always present (empty when unset), `flags` 257 (field 16), an unspecified kind as internal.

## 2026-11-06: feature: std/otlp, OTLP trace and metrics encoders

- Kind: feature
- Commit: this commit
- Found by: decision 0539 (OTLP export of std/trace and std/metrics)
- Go reference: opentelemetry-proto 1.x (trace.proto, metrics.proto, common.proto), checked against google.golang.org/protobuf marshaling the same messages
- Test: testdata/corpus/otlp (trace requests against protobuf-go), std/native/otlp_test.xo
- Shipped in: not yet released

`otlp.encode_traces(res, spans)` and `otlp.encode_metrics(res, registry, start_ms, now_ms)` write the OTLP request bodies
(see spec/std.md section 69). The HTTP exporter with retry follows.

## 2026-11-07: feature: otlp.Exporter, OTLP/HTTP with retry

- Kind: feature
- Commit: this commit
- Found by: decision 0539
- Go reference: the OTLP/HTTP specification (retry on 429, 502, 503, 504 with Retry-After; other statuses are permanent)
- Test: golden otlp_export (go, arm64, llvm)
- Shipped in: not yet released

`otlp.exporter(client, endpoint, res, clock, rand)` with `send_traces` and `send_metrics`, `with_headers` and `with_backoff`.

## 2026-11-07: feature: Retry-After as an HTTP date

- Kind: feature
- Commit: this commit
- Found by: follow-up of decision 0539
- Go reference: OTLP exporter retry rules (opentelemetry-go otlptracehttp: Retry-After seconds only; the date form is RFC 9110)
- Test: std/native/otlp_test.xo (retry-after: seconds, an HTTP date, ignored values)

The exporter waits for the larger of its backoff delay and the server's `Retry-After`, which may now be a number of seconds or an HTTP date (counted from the exporter's clock); a date in the past, a negative number or other text asks for no extra wait.
