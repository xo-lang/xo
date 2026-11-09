# 0580: OpenTelemetry-shaped spans in std/trace and std/otlp

Status: Proposed (Xo Lang 3, 2026-11-09; asked by Xo Azure 1 for tracing/azotel, notes/aws-43/PROPOSAL-std-trace-spans.md)

## Context

azotel (Go's `tracing/azotel` v0.4.0) maps each azcore span onto an OpenTelemetry span: a tracer per (namespace, version) with the
semconv schema URL, a parent from the context, a kind, attributes (set at start and later), events with attributes, a status
(unset, ok, error with a description), start and end timestamps, and attribute values of int, float, bool, string (anything else as
its `%v` text). std/trace (0325) keeps `Span{id, name, start_ms, micros, status}` in a `Ring`; std/otlp (0539) exports those with
ids derived from the request id, two kinds and no attributes. Nothing in std can carry an OpenTelemetry span, so azotel has nothing
to map onto.

## Decision (proposed)

1. **Grow `trace.Span`, do not add a second type.** Every new field has a default that means "as today", so existing literals
   compile and the Ring's output (`/debug/trace`-style JSON via `derive Json`) is unchanged for a span that leaves them at their
   defaults: `trace_id`, `span_id`, `parent_id` (hex, "" derives as otlp does today; `"root"` marks a root), `kind: SpanKind`
   (Unspecified keeps today's rule), `attributes: List[(Str, AttrValue)]`, `events: List[Event]`, `status_code: StatusCode` (Unset keeps
   today's HTTP-status rule) with `status_message`, `start_ns` and `end_ns` (Unix nanoseconds when > 0), and the instrumentation scope
   (`scope`, `scope_version`, `schema_url`). Every new field is written `as ",omitempty"` (spec/core.md 2.9: left out when empty), so the Ring's JSON bytes do not change (a test pins that). An enum value is never empty, so `kind` and `status_code` are Options (`SpanKind?`, `StatusCode?`, `None` meaning today's rule) rather than enums with an Unspecified/Unset default; the encoder maps `None` to OTLP's 0. The alternative, a separate `OtelSpan` plus a second encoder, duplicates the encoder and
   splits the Ring; rejected.
2. **`enum SpanKind`, `enum StatusCode`, `enum AttrValue {Str, Int, Float, Bool}`, `struct Event`** with OTLP's numbers (kind 0 to 5,
   status 0 to 2).
3. **`trace.Recorder` and `trace.Active`**: `recorder(ring, clock, rand)`; `start(name, parent: (Str, Str)?, kind, attributes, scope,
   scope_version, schema_url) -> Active` (ids known at start: 16 and 8 random bytes, never all zero), `ids()`, `set_attributes` (the last
   value per key wins, as OpenTelemetry), `add_event`, `set_status`, `end()` (records into the Ring once). Effects are `clock` and `rand`
   only.
4. **std/otlp encodes the new fields** per opentelemetry-proto 1.x trace.proto: ScopeSpans grouped by (scope, version, schema_url)
   (a span with no scope uses the Resource's), Span trace_id 1, span_id 2, parent_span_id 4 (absent for a root), name 5, kind 6,
   start 7 and end 8 (fixed64), attributes 9, events 11, status 15; KeyValue and AnyValue (string 1, bool 2, int 3, double 4); Event
   (time 1, name 2, attributes 3); Status message 2 only with Error (OpenTelemetry Go drops it for Ok and Unset), code 3.
5. **Proof**: Xo Azure 1's Go oracle (OpenTelemetry Go SDK with a fixed IDGenerator and explicit timestamps, exporting through
   otlptracehttp to a loopback fake) compared as decoded protojson with std/protojson; plus a vector that pins today's output for a
   span with default fields.

## Consequences

- azotel's step 2 (azure decision 0013) can build `new_tracing_provider(rec)` over the Recorder.
- `Span` grows by about a dozen defaulted fields: a Ring slot is larger; the Ring keeps its capacity in spans, so memory per Ring
  grows (measure `bench/perf` trace rows before and after; the new fields default to empty lists and strings, which cost no
  allocation).
- No existing code changes; `derive Json` output of an old-style span is byte identical.
- One new module surface (Recorder) in std/trace; std/changes entries for trace and otlp.

## Alternatives

- A separate `trace.OtelSpan` and `otlp.encode_otel_traces` (two encoders, two rings); rejected above.
- An OpenTelemetry-style `Tracer` interface instead of a Recorder struct: closures over the Ring are enough for azotel and
  azcore already carries its own Tracer; deferred.
- Attribute values as `Any`: OTLP's AnyValue also has arrays, maps and bytes; the scalar four are what azotel produces (`%v` text
  for the rest), so arrays and maps wait for a use.
