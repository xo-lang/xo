# std/htmltok changes

## 2026-11-07: port: std/htmltok, golang.org/x/net/html's Tokenizer

- Kind: port
- Commit: this push
- Found by: the Polar Bear Blog port (its bluemonday and html2text ports consume this tokenizer), decision 0548
- Go reference: golang.org/x/net v0.62.0 html (token.go, escape.go unescape)
- Test: internal/stdtest/testdata/corpus/htmltok (about 2,000 inputs in the plain, AllowCDATA and fragment modes and with buffer limits, token streams and raw texts against Go) on arm64, LLVM and the Go backend; `xo test --std-module=native/htmltok`
- Timing: host 1 best of 7, Xo against x/net v0.62.0 Tokenizer on a 12 KB page tokenized 2,000 times (bench/perf/htmltok): next 174 ms against 96 ms (1.8x Go), next plus token 287 ms against 207 ms (1.4x); the first port ran 328 ms and 450 ms (3.3x and 2.1x) until the scans of text, tag names, attribute keys and values and white space ran over a local index when there is no buffer limit (profile: tokenizer_rb 54% of the samples)
- Shipped in: not yet released

`htmltok.tokenizer(text)`, `next`, `token`, `raw`, `err` (spec/std.md 71). One source (std/native/htmltok.xo) for the native backends and, through std/go_compiled.txt, the Go backend. std/native/html.xo exports its entity lookup (`entity_of`, `entity2_of`, `REPLACEMENT`) for the attribute and text unescape, which differs from html.UnescapeString (x/net's attribute rule and numeric reference limits).
