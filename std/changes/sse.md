# std/sse changes

## 2026-11-16: perf: the parser finds LF line ends by search and decodes valid text once

- Kind: perf
- Commit: this push
- Found by: task #87 (bench/perf/sse parse 1.056 of Go on the Mac), XO_PROF=cpu: the byte loop of feed 29%, utf8's ASCII loop then to_str_lossy's validation
- Go reference: github.com/tmaxmax/go-sse v0.12.0 (the twin)
- Test: std/sse_test.xo (the line limit on LF-only chunks, LF-only and CR chunks mixed with a split CRLF, WHATWG decoding of text that is not UTF-8, besides the wpt cases fed whole and a byte at a time); bench/perf/sse on the Mac: parse instructions 3.26G to 2.77G (0.850), encode unchanged
- Shipped in: not yet released

`Parser.feed` moves from line to line with a search when the chunk holds no CR, takes the chunk without a copy when nothing is buffered, and keeps a valid UTF-8 field value as it is (one validation). Events unchanged.

## 2026-11-15: behavior: the web-platform-tests eventsource parsing cases

- Kind: behavior
- Commit: this push
- Found by: scripts/stdaudit checklist item sse.behavior.wpt (decision 0655)
- Go reference: none (WHATWG HTML 9.2.6); web-platform-tests eventsource/format-*.any.js at ddd4ca339ca0 (3-clause BSD license)
- Test: std/sse_test.xo, 16 tests over the 19 parsing cases (BOM, BOM split, comments, field parsing, data, event, id and Last-Event-ID, retry and its bogus and empty forms, unknown fields, leading space, newlines, U+0000, UTF-8), each body fed whole and one byte at a time (arm64, llvm)
- Shipped in: not yet released

No change to the parser: every case already gave the events the WPT tests expect. The tests now hold it.

## 2026-11-15: feature: `SseErr` is an open enum (decision 0649)

- Kind: feature
- Commit: this commit
- Found by: decision 0649 (a new variant of a std error enum broke two SDKs, 0644's `ClientErr.Tls`)
- Go reference: none (Go's errors are values compared with errors.Is; Rust's `#[non_exhaustive]`)
- Test: compiler/check's open enum test; the open_enum golden; xo check over the programs that use the module

`SseErr` is declared `open`: a `match` on it outside std needs a `_` (or
binding) arm, warning XO0306 in 0.52 and an error after, so that std can
add a variant in a minor release without breaking a build. Matches that
already have `_` are unchanged.

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
