# std/html changes

## 2026-10-30: perf: escape_string and unescape_string copy runs between the special bytes

- Kind: perf
- Commit: this commit
- Found by: the std timing audit (escape 3.818 and unescape 3.902 of Go's wall time, 2026-10-29)
- Go reference: html.EscapeString (a strings.Replacer), UnescapeString (runs copied between `&`)
- Test: std/native/html_test.xo (Go's cases, the round trip), bench/perf/html twincheck

The port walked every byte into a BytesBuilder (gotchas/std-port-costly-go-idioms.md).
Now escape_string has Go's byteStringReplacer shape: a first pass counts the extra bytes
the five specials need (a text without them is returned as it is), then one pass by index
into a BytesBuilder sized for the result copies each run and its entity. unescape_string
scans to each `&` by index into a builder sized to the input (the result is never longer),
takes `&lt;` `&gt;` `&amp;` `&#34;` `&#39;` and `&quot;` by a byte compare before the
general numeric and named entity path, and copies the run before each `&` whole. A first
version with find_any (the special set rebuilt per call, 52% of the profile) and the
entity binary search for every entity (its memcmp 25% of unescape; the fast path now
skips it for the six common ones) stayed at 2.79 and 4.02 of Go's wall time. Timing row (quiet `xo bench --ab`, 15 rounds, A the Go twin, B the Xo program, bench/perf/html, Xo / Go wall): escape 2.523 (2.502 to 2.537; 310.7 against 123.1 ms; instructions 2.392), unescape 0.617 (0.612 to 0.623; 187.9 against 304.7 ms; instructions 0.745), none 0.837; from the audit's 3.818 and 3.902. Escape stays above 1.2 of Go: its next step is the audit's note (bench/results/std-go-timing-audit-2026-10-26.md, html).

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
