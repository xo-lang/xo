# std/html changes

## 2026-11-03: perf: escape_string writes each replacement byte by byte

- Kind: perf
- Commit: this commit
- Found by: the Mac profile of bench/perf/html escape: `rs_append`, `move_at` and `rs_concat` (the `write_str` of a literal replacement) were 29 percent beside the scan loop
- Go reference: html escape.go EscapeString (go1.26.1)
- Test: bench/perf/html (twincheck), `xo test --std-module=html`
- Timing: Mac iteration (not evidence): escape 236 to 193 ms (Go 139); host row follows in std-twin-html-2026-11-03.md

The five replacements (`&amp;`, `&lt;`, `&gt;`, `&#39;`, `&#34;`) are written with `write_byte` instead of `write_str` of a literal Str, which went through the string append path (a move and a concat) for 4 to 5 bytes.

## 2026-11-03: perf: escape_string counts with a table and returns its bytes as the Str they are

- Kind: perf
- Commit: this commit
- Found by: bench/perf/html escape 2.31 of Go's wall on host 1 (bench/results/std-twin-rows2-2026-11-03.md)
- Go reference: html escape.go EscapeString (go1.26.1)
- Test: bench/perf/html (twincheck), `xo test --std-module=native/html`

The first pass adds each byte's extra length from a 256 byte table instead of five comparisons, and the
built bytes are returned as a Str without a UTF-8 check (they are ASCII replacements between runs of a
valid text). Xo / Go wall of escape: 2.31 to 1.75 (quiet `xo bench --ab`, host 1, 15 rounds).

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
