# std/xml changes

## 2026-11-03: feature: Reader, tokens one at a time

- Kind: feature
- Commit: this commit
- Go reference: encoding/xml Decoder.Token and RawToken (go1.26.1)
- Test: internal/stdtest/testdata/corpus/xml (a Reader against tokens and raw_tokens on every document, go, arm64, LLVM), xmlfuzz (every fuzz case reads through a Reader too), bench/perf/xml stream

`xml.reader(text, strict: true)`, `xml.raw_reader(text)` and `Reader.next() -> Token? ! XmlErr` (decision 0463): the tokens of `tokens` and `raw_tokens` one at a time, None at the end, the same error again after a failure. `tokens` and `parse` are unchanged. bench/perf/xml `stream` against `tokens`, host 1 (std-twin-rows8-2026-11-03.md): peak memory 0.70 of Go's against 7.2 for the list, wall 1.18 against 1.17.

## 2026-11-03: perf: character data with a standard entity stays on the fast path

- Kind: perf
- Commit: this commit
- Found by: bench/perf/xml (new twin, bench/results/std-twin-rows-2026-11-03.md): tokens 1.57x Go's wall on host 1; XO_PROF=cpu showed Dec.getc 21% and Dec.text 14% flat
- Go reference: encoding/xml xml.go Decoder.text (go1.26.1)
- Test: internal/stdtest/testdata/corpus/xml and xmlfuzz (go, arm64, LLVM), TestXmlFuzz

Character data and attribute values of printable ASCII and the five standard entities (`&lt;`, `&gt;`,
`&amp;`, `&apos;`, `&quot;`) are read in the fast path (text_entities); any other byte (a numeric or
other entity, `]`, `\r`, a byte over 126) still takes the byte by byte path from the same position, so
results and errors are unchanged. Quiet `xo bench --ab` on host 1, 15 rounds, Xo / Go wall: tokens 1.572
to 1.148, tree 1.245 to 0.945.

## 2026-10-27: feature: escape_to appends Go's EscapeText of a string to a builder

- Kind: feature
- Commit: this commit
- Found by: derive Xml's encoder writing into one builder (std/changes/xmlcodec.md 2026-10-27)
- Go reference: encoding/xml xml.go EscapeText (it writes to an io.Writer) (go1.26.1)
- Test: internal/stdtest/testdata/corpus/xmlcodec (escape_to of every piece text against escape; go, arm64, LLVM)

New: `xml.escape_to(out, s)` appends the text `xml.escape(s)` returns to
the StrBuilder `out`, with no string of its own.

## 2026-10-27: perf: encode keeps no stack of open tags

- Kind: perf
- Commit: this commit
- Found by: bench/perf/xmlcodec encode's CPU profile after the escape runs (Printer.pop_prefix 8.3%, write_end 6.7%, write_start 6.2%)
- Go reference: encoding/xml marshal.go (printer.writeStart, writeEnd, markPrefix, popPrefix) (go1.26.1)
- Test: internal/llvm/testdata/golden/xml_basic and xml_derive (arm64, LLVM); internal/stdtest/testdata/corpus/xml and xmlcodec (go, arm64, LLVM); twincheck bench/perf/xmlcodec; an encode differential (attribute prefixes declared on siblings, reused when nested, numbered on a clash, the xml prefix, an empty name, a comment holding -->) equal to the earlier std on go, arm64 and llvm

Element.encode writes each end tag from the element it started, so Go's
checks of an end tag against a stack of open tags cannot fail and the
stack is gone; the attribute prefixes an element declares are undeclared
back to a length kept for it, where a "" marker was pushed and popped
for every element. Output is unchanged. Quiet --ab, 15 rounds, the
parent's std as A: bench/perf/xmlcodec encode wall 0.801 (0.795 to
0.805), instructions 0.835.

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
