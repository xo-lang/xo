# std/xml changes

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
