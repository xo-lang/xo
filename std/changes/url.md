# std/url changes

## 2026-10-23: fix: a postgres URL keeps its comma-separated host list, as Go 1.26's parseHost

- Kind: fix
- Commit: this commit
- Found by: differential test against Go (internal/stdtest/testdata/corpus/httpurl, Go's url_test.go inputs)
- Go reference: net/url url.go parseHost, url_test.go urltests (go1.26.1, go.dev/issue/75223, 75859)
- Test: internal/stdtest/testdata/corpus/httpurl (TestStdCorpora, arm64 and LLVM)

`postgres://host1:1,host2:2,host3:3` failed with `invalid port ":1,host2:2,host3:3"
after host`. Go 1.26 keeps strict colon checking except for the postgres and
postgresql schemes, where the last colon starts the port. neturl.parse_host
now takes that case.

## 2026-10-22: perf: parse_query copies its keys and values of a long query

- Kind: perf
- Commit: 4438a0bf0
- Found by: memory probe (300 queries of 1 MiB, one kept key: 318 MB retained)
- Go reference: net/url url.go parseQuery (go1.26.1)
- Test: internal/llvm/testdata/golden/views_parsers (arm64, LLVM, go)

Slices are views (decision 0330), so a kept key or value of a long query
held all of it. Over 4 KiB each key and value is copied: 318 MB to 6 MB in
the probe; a shorter query keeps slices (bench/results/views-parsers-clone-2026-10-22.md).
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
