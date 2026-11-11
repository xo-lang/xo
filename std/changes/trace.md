# std/trace changes

## 2026-11-11: fix: Span's and Event's wire-named fields are public again

- Kind: fix
- Commit: this push
- Found by: the lead (golden trace_spans failed to build on arm64 after 1b13a8dbb)
- Go reference: none
- Test: goldens trace_spans, trace_ring, otlp_export (arm64, llvm, Go parity); TestStdCorpora trace, otlp
- Shipped in: not yet released

The merge (1b13a8dbb) made Span's trace_id, span_id, parent_id, kind,
attributes, events, status_code, status_message, start_ns, end_ns, scope,
scope_version and schema_url, and Event's attributes, private: programs
reading them failed with XO0605. They are `pub` again, as before the merge.

## 2026-11-11: refactor: std/trace is one file (decisions 0588, 0597)

- Kind: refactor
- Commit: this commit
- Found by: decision 0597's merges
- Go reference: none (no behavior change)
- Test: TestStdCorpora trace and otlp on go, arm64, llvm; xo test --std-module=std/trace (arm64, llvm); TestStdChecks
- Shipped in: not yet released

std/native/trace.xo moved into std/trace.xo (its tests into std/trace_test.xo). It is generated into std/rt (std/rt_hosted.txt), since std/otlp, which is, uses it.

## 2026-11-09: feature: OpenTelemetry span fields and a Recorder (decision 0580)

- Kind: feature
- Commit: this push
- Found by: Xo Azure 1 (tracing/azotel needs ids, parents, kinds, attributes, events, status and nanosecond times)
- Go reference: the OpenTelemetry Go SDK's span (sdk/trace), checked through std/otlp's oracle
- Test: std/native/trace_test.xo (a default Span's JSON is the old bytes; a full Span round-trips; a Recorder nests, merges attributes and records once; status rules and attribute order against the OpenTelemetry Go oracle)
- Shipped in: not yet released

`trace.Span` gains defaulted `omitempty` fields (ids, parent, kind, attributes, events, status, ns times, scope) and the module gains `SpanKind`, `StatusCode`, `AttrValue`, `Attr`, `Event`, `Recorder` and `Active`. The Ring and its JSON are unchanged for spans that set none of them. `Active.set_status` follows OpenTelemetry Go: Unset changes nothing, Ok is final and drops its message, Error keeps the last message.

## 2026-10-30: perf: a ring past its cap no longer copies every span on each record

- Kind: perf
- Commit: (this commit)
- Found by: Xo Lang 1's count of Mutex.with sites (decision 0431's data): trace.push was the one std hot path that paid for a copy
- Go reference: none (std/trace has no Go counterpart)
- Test: internal/llvm/testdata/golden/trace_ring (the ring bound, newest first; arm64, LLVM, Go parity)
- Timing: bench/perf/trace record 200000 1000 (200,000 spans on a ring of 1,000), xo bench --ab on the linux/arm64 offload host, 10 rounds, the parent as A: wall 0.156 (0.155 to 0.156), 4439 to 691 ms; peak RSS within noise. On the build before a81de74f2 (Mutex.with copying the list on each push) the same change gave 0.877

Once a ring held `cap` spans, every `record` and every request the
middleware saw sliced the list down to its newest `cap`, a copy of
`cap` spans per span. The list now grows to twice the cap and then
drops its older half, so a span costs one copy amortized, and `spans`
reads only the newest `cap`. The trim runs before the push, so no write
inside the `with` is followed by a point that can fault.

## 2026-10-21: feature: std/trace, request ids and a ring of spans

- Kind: feature
- Commit: dd2628eb7
- Found by: decision 0325 (the owner's language poll)
- Go reference: none (the shape of OpenTelemetry's span and a request id middleware); the hand written corpus follows decision 0312's addendum
- Test: internal/llvm/testdata/golden/trace_ring (ids from the header or random, the answer header, request and program spans, the ring bound, the JSON handler, auth; arm64, LLVM, Go parity)

`ring.middleware(rand, clock)` gives each request an id (the
`X-Request-Id` header or a random one) and records a `request` span;
`ring.record` adds the program's own; `ring.handler(auth)` serves the
last spans as JSON. One source for every backend (std/go_compiled.txt).
