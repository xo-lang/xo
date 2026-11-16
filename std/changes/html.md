# std/html changes

## 2026-11-16: perf: character references resolved with fewer compares and no allocation for ASCII

- Kind: perf
- Commit: this push
- Found by: task #87 (std/evidence/htmlparse/perf.md, ent 1.136 of Go on host 1), XO_PROF=cpu on bench/perf/htmlparse ent: htmlent.search 33%, the numeric reference's builder and UTF-8 check
- Go reference: golang.org/x/net/html token.go and escape.go (unescapeEntity; Go's entity map is a hash)
- Test: std/htmlent_test.xo (ENTITY_AT against ENTITY, every name found, names outside the table), std/htmltok_test.xo (numeric references below 128 and around it); bench/perf/htmlparse ent on the Mac: instructions 7.50G to 6.24G (0.832), parse and tok unchanged
- Shipped in: not yet released

`unescape_string` looks a named reference up among the entity names that share its first letter (std/htmlent's ENTITY_AT, about 6 compares where the whole table took 11). Output unchanged.

## 2026-11-14: perf: unescape_string's entity lookups search less

- Kind: perf
- Commit: this push
- Found by: the decision 0616 std/html review (a perf item: `&ampx` legacy prefixes ran 3.6 times Go on the Go backend, 17 times Go native on the Mac)
- Go reference: html.UnescapeString looks names up in its entity maps
- Before: entity2_of scanned all 93 two character entities for every reference that was not a one character entity, and entity_of's binary search over 2,138 names bound each (name, character) pair, copying and dropping both Strs a step
- Fix: entity2_of returns at once for a name without `;` (every two character entity has one) and binary searches otherwise; a name without `;` is looked up in LEGACY, the 106 names a reference may use without its semicolon (std/htmlent_test.xo checks it is exactly ENTITY's subset); the searches compare the name field alone, one compare a step. 2.4 MB of `&ampx` unescaped three times: 930 ms to 165 ms on the Mac (Go 48 ms); bench/perf/html unescape on linux/arm64 host 1, --ab against 7c0857749: 0.893 (0.881 to 0.898, n=11; 155.1 to 138.6 ms).
- Test: std/htmlent_test.xo (both tables sorted, LEGACY the subset, every entity found); xo test --std-module=html, htmlent and htmltok on native and llvm; TestStdCorpora htmlfuzz, htmltok, htmlparse on go, arm64, llvm
- Shipped in: not yet released

## 2026-11-14: security, fix: unescape_string answers a numeric reference of any length as Go, and follows Go's three byte rule

- Kind: security, fix
- Commit: this push
- Found by: the new std/html fuzz harness (decision 0616, internal/stdtest/htmlfuzz_test.go, seed 1 against go1.26.11)
- Go reference: html escape.go unescapeEntity: the digits accumulate in a rune (int32), which wraps, and nothing matched when the reference ends within three bytes of its `&` (`if i <= 3`)
- Security: a long numeric character reference faulted on Int overflow, so unescaping a peer's text stopped the program, or answered 500 in an http handler; now its digits wrap as Go's rune does, and a reference of any length is read in one pass
- Fix: `&#x;` is U+FFFD and `&#9` before another byte stays text, as in Go (Xo unescaped any digit and kept `&#x;`); 215 of the fuzz's 2,000 seed 1 cases differed before
- Test: std/html_test.xo "a numeric reference past Int answers as Go's rune does, not a fault" and "Go's three byte rule for numeric references"; corpus internal/stdtest/testdata/corpus/htmlfuzz on every corpus backend; TestHtmlFuzz seed 2 on arm64 and llvm
- Affected: the fault on the native backends from 0.35.0 (std/html's first release) to 0.51.0, and on the Go backend from 0.49.0 (unescape_string compiled from std/html.xo, which wrapped to a different answer there rather than faulting); the three byte rule on every backend in those releases
- Shipped in: 0.51.1

## 2026-11-10: internal: merged, escape_string's fast path in std/native/html_native.xo

- Kind: internal
- Commit: this push
- Found by: decisions 0588 and 0597 part 2 (one file per std module, native parts behind bodyless declarations)
- Go reference: html escape.go (go1.26.1)
- Test: `xo test --std-module=html` (native, LLVM); the htmltok and htmlparse corpora on go, arm64, llvm; bench/perf/html checksums equal on arm64, llvm, go
- Timing: none (the code is the same); the portable escape_string measured 1.674 of this (1.659 to 1.692, n=6, host 1) and was not taken
- Shipped in: not yet released

std/html.xo holds the module with unescape_string's body and its tests (std/html_test.xo); escape_string is declared there and its body is std/native/html_native.xo's (Go's html.EscapeString on the Go backend). std/native/html.xo and the Go backend's hand Html_unescape_string are removed. Output unchanged.

## 2026-11-10: internal: the entity tables move to the merged std/htmlent

- Kind: internal
- Commit: this push
- Found by: decision 0588 (one file per std module); htmlent blocked merging std/html and std/htmltok (XO0601)
- Go reference: html entity.go (go1.26.1)
- Test: `xo test --std-module=html` and `htmltok` (native, LLVM); the htmltok and htmlparse corpora on go, arm64, llvm
- Timing: none (no code change)
- Shipped in: not yet released

std/native/htmlent.xo is removed; its tables and lookups are std/htmlent.xo's, and the html and htmltok natives `use std/htmlent`. Output unchanged.

## 2026-11-08: perf: entity_of answers the common references before the search

- Kind: perf
- Commit: this push
- Found by: a host 1 profile of std/htmltok (entity_of 16% of the samples), which shares the lookup
- Go reference: html entity.go (go1.26.1)
- Test: `xo test --std-module=native/html`; the htmltok and htmlparse corpora
- Timing: host 1 A/B 0.879 of the parent (0.874 to 0.89, n=6; bench/perf/htmlparse `tok`, a page with `&amp;`, `&copy;`, `&lt;` and `&gt;`); the general case, a page of only uncommon references (bench/perf/htmlparse `ent`, none of the seven answered first): 1.017 (1.012 to 1.03, n=7; 538.0 to 547.3 ms), the cost of the seven failed compares
- Shipped in: not yet released

`amp;`, `lt;`, `gt;`, `quot;`, `apos;`, `nbsp;` and `copy;` return their table values from a match before the binary search of the entity table; the results are the table's.

## 2026-11-07: perf: escape_string sizes 32 bytes a step

- Kind: perf
- Commit: this push
- Found by: ADR 0532 step 2's candidates (html escape sizing)
- Go reference: html escape.go EscapeString (go1.26.1)
- Test: `xo test --std-module=native/html` (a new case: each special at each place of 0 to 100, with a special every 37 bytes, against the expected text)
- Timing: host 1 A/B escape 0.871 of main (0.864 to 0.887, n=6; bench/perf/html, a special every 5 bytes)
- Shipped in: not yet released

The size scan counts the five specials of 32 bytes with five lane compares (rt.bytes_count_eq32), then finishes a word a step as before.

## 2026-11-07: fix: unescape_string made `&quot;` a backslash and a quote

- Kind: fix
- Commit: this push
- Found by: the std/htmltok token corpus against x/net (an attribute value `&quot;` read back as two bytes)
- Go reference: html escape.go UnescapeString (go1.26.1)
- Test: `xo test --std-module=native/html` (the `quot` case of Go's list: `&quot;`, `&QUOT;` and their semicolon-less forms)
- Timing: not affected
- Shipped in: not yet released

The entity table held a backslash and a quote (two bytes) for `quot` and `QUOT` where Go's table has one `"`; the table moved to std/native/htmlent.xo (shared with std/htmltok) with the four entries fixed.

## 2026-11-06: perf: escape_string scans 8 bytes a step

- Kind: perf
- Commit: this push
- Found by: the SIMD survey of std's byte paths (bench/results/simd-survey-2026-11-06.md)
- Go reference: html escape.go EscapeString (go1.26.1)
- Test: `xo test --std-module=native/html` (a new case: each special at each place of 0 to 40, and near misses)
- Timing: host 1 A/B escape 0.943 of before (0.91-0.947, n=5) on bench/perf/html's text, a special every 5 bytes; a first version that rechecked the word at each byte was 2.44 of before on it
- Shipped in: not yet released

The size scan counts each word's specials by mask (4 extra bytes for `&`, `'` and `"`, 3 for `<` and `>`), and the write loop skips a word with no special and otherwise goes straight to its first one.

## 2026-11-04: perf: escape_string writes the output through its address

- Kind: perf
- Commit: this push
- Found by: bench/perf/html escape 1.37 of Go's wall after the byte-by-byte change
- Go reference: html escape.go EscapeString (go1.26.1)
- Test: `xo test --std-module=native/html` on arm64 and LLVM (a new case with adjacent and trailing specials and multi-byte runs), bench/perf/html twincheck
- Timing: host 1 A/B escape 0.846 of before (0.835-0.853, n=6), about 1.16 of Go's wall
- Shipped in: not yet released

The output buffer is exactly sized (the first scan counts the extra bytes) and written with one `mem_copy` for each run of unescaped bytes and one 4 byte store for each replacement (a `;` byte after for `&amp;`, `&#39;` and `&#34;`), in place of a builder call for each byte.

## 2026-11-03: perf: escape_string writes each replacement byte by byte

- Kind: perf
- Commit: this commit
- Found by: the Mac profile of bench/perf/html escape: `rs_append`, `move_at` and `rs_concat` (the `write_str` of a literal replacement) were 29 percent beside the scan loop
- Go reference: html escape.go EscapeString (go1.26.1)
- Test: bench/perf/html (twincheck), `xo test --std-module=html`
- Timing: host 1 (bench/results/std-twin-html-2026-11-03.md): escape 1.75 to 1.37 of Go, unescape 0.54; Mac iteration: escape 236 to 193 ms (Go 139)

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
