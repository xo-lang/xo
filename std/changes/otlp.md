# std/otlp changes

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
