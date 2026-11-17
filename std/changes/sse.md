# std/sse changes

## 2026-11-16: fix: a reconnection sends the last event ID as of the last blank line, not an id line of an event the break cut off

- Kind: fix
- Commit: this commit
- Found by: azure-1 (the sse stress, task #115: a subscriber lost the event a break cut after its `id` line)
- Go reference: none in Go's standard library; the HTML Living Standard 9.2.6 (dispatch, step 1: the event source's last event ID string is set from the buffer at a blank line), as browsers' EventSource and github.com/tmaxmax/go-sse keep it
- Test: std/sse_test.xo "a reconnection asks after the last dispatched event, not after an id the break cut off"; std/evidence/sse/stress; internal/stdtest TestSseFuzz (the parser against the standard's algorithm)
- Shipped in: not yet released

`Source` (and `subscribe`) reconnected with the parser's last event ID buffer, which an `id` line sets
at once: a stream cut after an event's `id: N` line and before its blank line reconnected asking for
the events after N, so event N, never dispatched, was lost; and an `id` of a later event in the same
chunk could be sent too. It now sends the id of the last event it returned, or at a break the buffer
as of the latest blank line, an id-only one (no data) included, as the standard's event source does.
`Parser.dispatched_id` and `Reader.dispatched_id` give that id.

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

## 2026-11-17: perf: an event's data lines are decoded once, joined

- Kind: perf
- Commit: this commit
- Found by: task #88 (bench/perf/sse parse 1.077 of Go's time on linux/arm64 host 1)
- Test: std/sse_test.xo (a UTF-8 sequence cut by a line end is one U+FFFD and the next data line does not complete it); host 1 `xo bench --ab` bench/perf/sse parse: 0.883 of before (wall 130.5 to 115.2 ms)
- Shipped in: not yet released

A data line is appended to one byte builder per event and the event's data is decoded at the blank line, where
each line was decoded to a Str, kept in a list and joined. An LF ends any invalid sequence, so the data is the same.
