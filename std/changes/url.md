# std/url changes

## 2026-10-21: fix: unescape errors and parse_query's semicolon error as Go's

- Kind: fix
- Commit: 60c8372e
- Found by: differential test against Go (scripts/stdcorpus url: 1,500 escape texts, 1,200 queries)
- Go reference: net/url url.go EscapeError.Error, parseQuery (go1.26.1)
- Test: internal/stdtest/testdata/corpus/url

Natively, an escape cut inside a rune was quoted with `%XX` in the error
(`invalid URL escape "%#%C3"`); Go's strconv.Quote writes `"%#\xc3"`.
parse_query returned the first escape error even when a later pair held a
semicolon. Go reports `invalid semicolon separator in query` then,
because that error replaces any earlier one.

## 2026-10-20: port: std/url, Go's query and path escaping and url.Values

- Kind: port
- Commit: 3c9839e6
- Found by: survey
- Go reference: net/url/url.go (go1.26.1)
- Test: std/native/url_test.xo (Go's url_test.go tables); internal/llvm/testdata/golden/url_basic

New module: `query_escape`, `query_unescape`, `path_escape`,
`path_unescape`, and `Values` with `parse_query` and `encode`, with Go's
results and error texts on every backend. Bytes that decode to invalid
UTF-8 stay escaped, so the results are valid `Str`.
