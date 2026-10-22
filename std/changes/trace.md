# std/trace changes

## 2026-10-21: feature: std/trace, request ids and a ring of spans

- Kind: feature
- Commit: this commit
- Found by: decision 0325 (the owner's language poll)
- Go reference: none (the shape of OpenTelemetry's span and a request id middleware); the hand written corpus follows decision 0312's addendum
- Test: internal/llvm/testdata/golden/trace_ring (ids from the header or random, the answer header, request and program spans, the ring bound, the JSON handler, auth; arm64, LLVM, Go parity)

`ring.middleware(rand, clock)` gives each request an id (the
`X-Request-Id` header or a random one) and records a `request` span;
`ring.record` adds the program's own; `ring.handler(auth)` serves the
last spans as JSON. One source for every backend (std/go_compiled.txt).
