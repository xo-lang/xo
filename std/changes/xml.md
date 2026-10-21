# std/xml changes

## 2026-10-21: feature: derive Xml, Go's Marshal and Unmarshal for structs

- Kind: feature
- Commit: (filled in after the push)
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
