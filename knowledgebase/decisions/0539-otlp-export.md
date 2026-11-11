# 0539: OTLP export for std/trace and std/metrics

Status: Accepted (owner, 2026-11-11; implemented on main). Was: Proposed (Xo Lang 3, 2026-11-06)

## Context

The owner queued OTLP export after protobuf (0530) and gRPC (0535). std/trace keeps a ring of `Span{id, name, start_ms, micros, status}`
and has no exporter; std/metrics keeps a Registry of counters, gauges, histograms and summaries and renders the Prometheus text. A
collector (OpenTelemetry Collector, Jaeger, Tempo, Azure Monitor) takes OTLP: protobuf messages over HTTP (`POST /v1/traces`,
`/v1/metrics`, `application/x-protobuf`) or gRPC (`TraceService/Export`, `MetricsService/Export`).

## Decision (proposed)

1. A new module **std/otlp** holds the OTLP messages as `derive Proto` structs (opentelemetry-proto 1.x field numbers: the trace
   and metrics request trees, Resource, InstrumentationScope, Span, Status, Metric with Gauge, Sum and Histogram, number and
   histogram data points, KeyValue and AnyValue) and an exporter.
2. **Transport**: OTLP/HTTP with protobuf bodies first, over std/http's client; OTLP/gRPC second, over std/grpc. Retry as the OTLP
   spec says (429, 502, 503, 504 with `Retry-After`, exponential backoff; other statuses are not retried), and `Partial success`
   answers are reported, not retried.
3. **std/trace** gains what a trace needs: `Span` fields `trace_id` (16 bytes), `span_id` (8 bytes) and `parent_id`, all with
   defaults so existing code compiles; the middleware reads and writes the W3C `traceparent` header. The request id stays.
4. **std/metrics** maps Counter to a monotonic cumulative Sum, Gauge to Gauge, Histogram to a cumulative explicit bounds
   Histogram, Summary to Summary data points.
5. The exporter is a value (`otlp.exporter(client, endpoint, clock)`) with `send_spans`, `send_metrics` and a batching task
   (`run(group, interval)`); resource attributes (`service.name`, ...) are set once.

## Consequences

- No oracle exists offline (no opentelemetry-proto Go module in the cache): the check is golden bytes derived from the proto
  field numbers and read back by Go's protowire (an independent field and wire type walk), and a round trip through std/protoparse
  of the vendored .proto text to check the numbers.
- Adds fields to `trace.Span` (source compatible).

## Alternatives

- OTLP/JSON only: smaller to build, but collectors treat it as secondary and it hides field number mistakes.
- A generic OpenTelemetry API (Tracer, Meter) like the Go SDK's: much larger; the first version exports what std/trace and
  std/metrics already record.
