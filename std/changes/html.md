# std/html changes

## 2026-10-29: port: Go's html package (escape_string, unescape_string)

- Kind: port
- Commit: this commit
- Go reference: html/escape.go and entity.go (go1.26.1)
- Test: std/native/html_test.xo (Go's unescapeTests, the round trip, legacy names, invalid references)

The first version of the module: `escape_string` (the five characters) and
`unescape_string` (named entities with the 2138 one-character and 93
two-character names of Go's table, the legacy names without a semicolon by
longest prefix, decimal and hexadecimal references with the C1 and invalid
code point rules). The entity lookup is a binary search over the sorted
table. Timing row against Go's package: bench/perf/html (escape,
unescape), in the std timing audit with the html mode's rows.
