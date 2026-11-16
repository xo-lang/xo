# std/trace changes

## 2026-11-16: behavior: `recorder` and `Ring.middleware` take a `Rand[rand]` and `Recorder.rand` is a `Rand[rand]` (decision 0652)

- Kind: behavior
- Commit: this push
- Found by: task #141, Rand's effect parameter (decision 0652)
- Go reference: none (Xo's effect system)
- Test: std module tests; golden rand_pure
- Shipped in: not yet released

Rand takes its effect as a parameter now; these hold the OS generator, so they are written with the effect they had before. A `Rand[()]` (a pure one) is still accepted.

## 2026-11-16: perf: a Ring overwrites its oldest span in place

- Kind: perf
- Commit: this push
- Found by: task #88, the record row of bench/perf/trace over 1.05 (a profile: the copy of the newest half, its Str shares, and the drop of the old list were most of a record)
- Go reference: the twin's Ring (bench/perf/trace/twin.go.txt, a slice trimmed to its newest cap at twice the cap)
- Test: std/trace_test.xo (a ring past its cap keeps the newest spans, newest first, through many wraps, with the n and min filters)
- Timing: bench/perf/trace record 1.23 to 1.31 of Go before, 0.70 to 0.87 after (Mac, three runs each; host 1 row to follow)

A `Ring` kept its spans in a list that grew to twice its cap and then
copied its newest half into a new list. It now holds at most cap spans
and each new span overwrites the oldest one in place. `spans` returns
the same spans in the same order. `Ring`'s fields, which are internal,
changed (`buf` holds a `RingSlots`).

## 2026-11-16: perf: span and request ids come from Rand.int

- Kind: perf
- Commit: this push
- Found by: task #88, the span row of bench/perf/trace (2.9 of Go: each Rand.bytes is a system call)
- Go reference: go.opentelemetry.io/otel/sdk/trace randomIDGenerator (ids from a math/rand source seeded once)
- Test: std/trace_test.xo (ids are lowercase hex of their length, never all zero, and differ from draw to draw; the recorder test with --real) and bench/perf/trace (the twin's checksum)
- Timing: bench/perf/trace span 2.90 to 3.08 of Go before, 0.70 to 0.96 after (Mac, three runs each; host 1 row to follow)

A new trace id (16 bytes), span id (8 bytes) and the middleware's
request id (8 bytes) were drawn with `Rand.bytes`, a system call each.
They are now drawn with `Rand.int`, 32 bits a call, which reads the
userspace ChaCha8 source of decision 0669 on native builds. The ids keep
their form: lowercase hex, never all zero.

## 2026-11-16: perf: a root span draws both ids at once

- Kind: perf
- Commit: this push
- Found by: task #88 (performance Early to Proven), the module's first Go twin (bench/perf/trace span: 5.45 of Go on the Mac)
- Go reference: none in the standard library (the twin writes the ring and recorder out over crypto/rand)
- Test: the std/trace tests (with --real), the trace_spans golden; bench/perf/trace twincheck output matches Go
- Timing: bench/perf/trace span on the Mac, Xo over Go: 5.450 to 2.824 (twincheck -time 3)
- Shipped in: not yet released

Recorder.start drew the trace id and the span id from Rand separately, and each draw is a system call. A root span now takes its 24 bytes in one draw; a seeded test Rand gives the same ids as before.

## 2026-11-15: behavior: the middleware and handler types allow `sync`

- Kind: behavior
- Commit: this push
- Found by: decision 0662 (a closure that captures shared state performs `sync`)
- Go reference: none
- Test: test/errorcheck/effect_sync_capture; the module's own tests check clean
- Shipped in: not yet released

`Ring.middleware` returns `fn(http.Request, http.Next) -> http.Response uses (rand, clock, sync)` and `Ring.handler` returns `fn(http.Request) -> http.Response uses sync`: both closures use the ring's Mutex, which a pure function type now refuses (decision 0662). `router.wrap` and `router.handle` take them as before; a program that stored one in a variable of the old type adds `sync` to that type.

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
