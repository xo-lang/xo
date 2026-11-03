# 0463: A streaming token reader for std/xml (proposal)

Status: Accepted (owner, 2026-11-03)

## Context

`xml.tokens(text)` returns every token as a `List[Token]`, and `xml.parse(text)` builds the whole
`Element` tree. Go's `xml.Decoder.Token` hands out one token at a time and `Unmarshal` fills structs.
bench/results/std-twin-rows-2026-11-03.md (host 1, 1.7 MB document of 20,000 items, 8 passes): Xo's peak
memory is 3.0x Go's for tokens (32.7 MB against 10.9 MB) and 2.1x for the tree (49 MB against 23 MB), while
wall time is 1.15 (tokens) and 0.95 (tree) of Go's. Every other fuzzed module is 0.5 to 1.0 of Go's memory.
The extra 22 MB for tokens is about 100 bytes a token over roughly 220,000 tokens a pass (derived, not
measured per token: a Start holds a Name and a List of Attr, each Name two Str views).

What is already streaming: the native decoder `Dec` (std/native/xml.xo) is incremental. `read_all` is a loop
over `d.token()` pushing into a list, and the tree builder consumes tokens the same way. The Go backend's
`xmlRead` (std/rt/xml.go) also collects `Decoder.Token` into a list. The input is a whole `Str` (a 1.7 MB
buffer, shared as views), so even a streaming reader holds the input; the floor is the input size (Go's
10.9 MB includes it too).

## Options

1. `xml.Reader`: `xml.reader(text: Str, strict: Bool = true) -> Reader`, `fn (var r: Reader) next() -> Token? ! XmlErr`
   (None at end of input), plus `raw_reader`. A `Reader` wraps `Dec`; `tokens` and `parse` stay as they are
   and become loops over it. Cost: a new opaque type in std/xml.xo (declaration file), the native
   implementation (about 30 lines: hold a Dec, return `d.token()` or the error), the Go backend's version
   (hold a `*xml.Decoder`; std/rt/xml.go and its generated signatures, `go run ./scripts/stdgen`), spec
   section 34, std/changes/xml.md, a corpus case per backend (go, arm64, LLVM) that a Reader yields the same
   tokens as `tokens`, TestXmlFuzz comparing Reader against Go's Token loop. wasm: the same native code.
2. Stream from an `io` source instead of a `Str` (Go's `NewDecoder(r io.Reader)`): lowers memory below the
   input size, but Dec reads `d.src` as `Bytes` by index everywhere (getc, text's fast paths, the views
   into `whole`), so it needs a refill window and views would have to copy or pin their window. Large change
   to the decoder and to decision 0330's view rule; not needed for the 3.0x figure.
3. Stream the tree into a caller's type (`decode[T: Xml]` already does for derive Xml: std/changes/xmlcodec.md).
   `parse` stays whole-tree; nothing to build.
4. Do nothing and document that `tokens` and `parse` hold the whole result (spec section 34 already says they
   return a list and a tree).

## Recommendation

Option 1, as an addition only: no existing function changes behaviour, so nothing breaks, and the measurable
effect is that a program that counts or filters tokens runs in the input's size plus one token. Expected
effect from the numbers above: peak memory of the tokens workload from 3.0x toward 1.0x of Go's (input plus
one token), wall time unchanged to slightly better (no list growth). Verify with bench/perf/xml, a new
`stream` workload (Go twin: the same Token loop it already has), `xo bench --ab` on host 1.

## Consequences

One more public type and function pair in std/xml (an API the owner decides: the name `Reader` and whether
`raw` is a flag or a second function). Option 2 stays open later because Reader hides the source.

## Alternatives

Options 2 to 4 above.
