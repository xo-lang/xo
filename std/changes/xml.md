# std/xml changes

## 2026-10-27: perf: encode copies the text between escapes whole

- Kind: perf
- Commit: this commit
- Found by: bench/perf/xmlcodec encode at 1.79x encoding/xml; its CPU profile (obj.rs_append 12%, move_at 6.9%, Printer.write_start 6.1%)
- Go reference: encoding/xml xml.go (EscapeText, escapeText: the run since the last escape is written in one call) and marshal.go (printer.writeStart, writes each piece) (go1.26.1)
- Test: internal/llvm/testdata/golden/xml_basic and xml_derive (arm64, LLVM); internal/stdtest/testdata/corpus/xml and xmlcodec (go, arm64, LLVM); twincheck bench/perf/xmlcodec; an escape and encode differential (control bytes, U+FFFE and U+FFFF, emoji, CJK, namespaces, with and without indent) equal on go, arm64 and llvm

Escaping writes each run of plain text between two escapes in one append,
and a text with nothing to escape whole, where it appended a string per
character. Start and end tags and attributes are written piece by piece
into the printer's builder instead of through string interpolation.
Output is unchanged. Quiet --ab, 15 rounds, the parent's std as A:
bench/perf/xmlcodec encode wall 0.687 (0.677 to 0.694), instructions 0.632.

## 2026-10-22: perf: parse reads spaces, names and plain text without a call per byte

- Kind: perf
- Commit: 9c3403e02
- Found by: XO_PROF=cpu (Dec.getc 19% and read_name, mustgetc 15%, the per-rune checks of text)
- Go reference: encoding/xml xml.go (Decoder.getc, name, text) (go1.26.1)
- Test: internal/stdtest/testdata/corpus/xml and internal/llvm/testdata/golden/xml_basic (arm64, LLVM, go)

`parse` and the token reader skip spaces in one loop, take an ASCII name
and plain character data (printable ASCII, tab, newline) as slices of the
text, and fall back to the byte by byte reading of Go's decoder for anything
else (an entity, `]`, `\r`, non-ASCII bytes, CDATA). A 1.7 MB document,
parsed 8 times, native arm64: 574 ms to 258 ms wall (`xo bench --ab`, 0.449,
interval 0.438 to 0.490); 8,004 M to 3,768 M instructions, Go's Token loop 3,760 M.
Results are the same.

## 2026-10-21: feature: derive Xml, Go's Marshal and Unmarshal for structs

- Kind: feature
- Commit: 03854dd2
- Found by: decision 0315 (owner choice B: expansion in the checker)
- Go reference: encoding/xml marshal.go (marshalStruct, the parent stack, marshalSimple) and read.go (unmarshal, copyValue) (go1.26.1)
- Test: internal/stdtest/testdata/corpus/xml (1,500 documents decoded and re-encoded); test/errorcheck/derive_xml

New: `xml.encode_value(v, indent)` and `xml.decode[T](text)` for a struct
with `derive Xml`, its fields tagged with `xml "..."` strings in Go's tag
text (`ID,attr`, `,chardata`, `a>b>c`, `,omitempty`, `-`). `Node` gains
`Raw(s)`, text `encode` writes as it is. The helpers are the new
std/xmlcodec, written once for every backend. Differences from Go are in
spec std 34.

## 2026-10-21: port: std/xml, Go's encoding/xml tokens and a tree

- Kind: port
- Commit: 2a2edc53
- Found by: survey (SOAP, SAML, sitemaps, TwiML, S3 bodies; decision 0314)
- Go reference: encoding/xml xml.go (Decoder: Token, RawToken, text, names, the HTML tables), marshal.go (EncodeToken, Indent) (go1.26.1)
- Test: internal/stdtest/testdata/corpus/xml; internal/llvm/testdata/golden/xml_basic

New module: `tokens` and `raw_tokens` give Go's Decoder tokens, with
namespaces resolved and Go's errors. `parse` builds an element tree,
with `attr`, `child`, `children_named`, and `text`. `encode` writes it as
Go's Encoder does, and `escape` is EscapeText. `strict: false` is Go's
HTML leniency.
