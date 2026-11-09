# OTel oracle for std/trace spans and std/otlp (std ADR 0580)

`main.go` drives OpenTelemetry Go's SDK (otel v1.47.0, sdk v1.47.0,
otlptracehttp v1.47.0, proto/otlp v1.11.1) with a fixed IDGenerator and
explicit timestamps, and exports each case through otlptracehttp (no
compression) to a loopback fake collector. One case is one provider
flushed once, so one request body.

Build and run (env -i, GOPATH and GOCACHE in a scratch home):

    go build -o PROG . && env -i PROG out && python3 -I canon.py out

## Files in out/, per case

- `<case>.bin`: the request body as sent (ExportTraceServiceRequest).
- `<case>.json`: that body as Go's protojson, keys sorted (canon.py).
  Bytes fields (traceId, spanId, parentSpanId) are base64, as Go's
  protojson writes them (not OTLP/JSON's hex); 64-bit ints are strings.
- `<case>.spec`: what the program asked for, in call order:
  - `scope N "name" "version" "schema_url"` (`""` = not set)
  - `span TRACE_ID SPAN_ID PARENT_SPAN_ID|root KIND "name" START_NS END_NS scope=N`
    (ids as hex; KIND unspecified|internal|server|client|producer|consumer)
  - `  attr "key" TYPE VALUE`: a start attribute (TYPE string|int64|float64|bool; VALUE is JSON)
  - `  set | "key" TYPE VALUE | ...`: one set_attributes call
  - `  event "name" TIME_NS`, then `    eattr "key" TYPE VALUE` lines
  - `  status Unset|Ok|Error "description"`: one set_status call
  Spans start in file order and end in file order (so a parent may
  end before its child; the body lists spans in end order, grouped by
  scope in first-seen order).

Resource: only `service.name` = `oracle` (resource.NewSchemaless), so
no SDK or host attributes and no resource schema URL.

Trace ids are `101112131415161718191a1b1c1d1e` plus a counter byte; span
ids `a0000000000000` plus a counter byte, counted per case in start order
(the IDGenerator restarts per case).

## What the SDK does (things std has to match)

- **Flags**: every span has `flags: 257` (Span field 16, fixed32):
  0x100 (SPAN_FLAGS_CONTEXT_HAS_IS_REMOTE_MASK) | 0x01 (sampled). A
  span whose parent is remote would add 0x200; none here.
- **Status is always present**: unset status is an empty Status message
  (`"status": {}`, bytes `7a 00`), not an absent field.
- **Status rules** (case status): Unset with a description is ignored;
  Ok drops its description; Ok is final (a later Error is ignored);
  Error then Ok becomes Ok; Error then Error keeps the last message;
  Error then Unset stays Error with its message.
- **Kind**: Unspecified is exported as INTERNAL.
- **Attributes**: last value per key wins, kept at the key's first
  position (case attribute-overwrite: a, b, c after `a` was replaced
  twice, once with a different type); duplicates at start keep the last.
- **Zero values are encoded**: `""`, `0`, `0.0` and `false` are written
  as set AnyValue oneofs (`stringValue: ""` etc.), never omitted.
- **Events** keep call order, duplicates and equal timestamps; an event
  without attributes has no attributes field.
- **Scopes**: one ScopeSpans per (name, version, schema URL) in
  first-seen order; version and schema URL omitted when empty.
- Order of fields on the wire follows field numbers (Go's proto
  marshal); compare decoded messages, not raw bytes, unless std writes
  in field order too.
