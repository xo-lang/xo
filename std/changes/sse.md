# std/sse changes

## 2026-11-11: feature: std/sse, server-sent events

- Kind: feature
- Commit: this commit
- Found by: decision 0603 stage 1 (Accepted, owner 2026-11-11)
- Go reference: none in Go's standard library; a WHATWG reference parser in scripts/stdcorpus/sseoracle, checked against the web-platform-tests eventsource format-* and event-data cases and github.com/tmaxmax/go-sse v0.12.0's sse.Read
- Test: internal/stdtest/testdata/corpus/sse (20 web-platform-tests streams, 60 seeded random streams, each whole, byte by byte and in random chunks; Reader, encode and comment cases) on arm64, LLVM and the Go backend
- Shipped in: not yet released

`sse.parser` and `feed` (I/O free), `reader` over any read function, `encode`, `comment`, `send` and `headers`
(spec/std.md 79). The azure and openai SDKs keep their readers for now; they move onto it in a later release.

## 2026-11-11: feature: sse.Source and sse.subscribe, reconnection

- Kind: feature
- Commit: this commit
- Found by: decision 0603 stage 1 (the client helper that reconnects with Last-Event-ID and the server's retry)
- Test: internal/stdtest/testdata/corpus/sse (reconnection with Last-Event-ID and retry over FakeNet and FakeClock; a refused stream; giving up after max_retries) on LLVM and the Go backend; arm64 waits on the "reads virtual register before it is written" fix
- Shipped in: not yet released
