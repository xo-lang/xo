# std/trace changes

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
