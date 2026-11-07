# std/htmltok changes

## 2026-11-07: port: std/htmltok, golang.org/x/net/html's Tokenizer

- Kind: port
- Commit: this push
- Found by: the Polar Bear Blog port (its bluemonday and html2text ports consume this tokenizer), decision 0548
- Go reference: golang.org/x/net v0.62.0 html (token.go, escape.go unescape)
- Test: internal/stdtest/testdata/corpus/htmltok (about 2,000 inputs in the plain, AllowCDATA and fragment modes and with buffer limits, token streams and raw texts against Go) on arm64, LLVM and the Go backend; `xo test --std-module=native/htmltok`
- Timing: not measured yet (row against Go's tokenizer to come)
- Shipped in: not yet released

`htmltok.tokenizer(text)`, `next`, `token`, `raw`, `err` (spec/std.md 71). One source (std/native/htmltok.xo) for the native backends and, through std/go_compiled.txt, the Go backend. std/native/html.xo exports its entity lookup (`entity_of`, `entity2_of`, `REPLACEMENT`) for the attribute and text unescape, which differs from html.UnescapeString (x/net's attribute rule and numeric reference limits).
