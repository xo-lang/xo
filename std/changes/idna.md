# std/idna changes

## 2026-11-03: feature: std/idna, internationalised domain names

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 backend survey (std-coverage.md row 12: Unicode normalization (NFC) and IDNA); decision 0476 (std/idna Accepted, owner 2026-11-03)
- Go reference: golang.org/x/net v0.60.0 idna (idna.go, punycode.go) and golang.org/x/text v0.43.0 secure/bidirule, BSD-style license
- Test: internal/stdtest/testdata/corpus/idna (about 1,000 names on the four profiles, ToASCII and ToUnicode, with the error code, compared with x/net; 31 punycode vectors of x/net's punycode_test.go; known names) on arm64, LLVM and the Go backend; bench/perf/idna twincheck against x/net
- Cost and timing: bench/results/std-idna-size-2026-11-03.md (size beside Go's, verdict) and bench/results/std-twin-rows16-2026-11-03.md (timing)

`idna.to_ascii`, `idna.to_unicode`, `idna.Profile`, `idna.IdnaErr`, `idna.unicode_version` (spec/std.md 61). Plain ASCII names (letters, digits, hyphen, dot) skip the table.

## 2026-11-03: perf: ASCII index, in place decoding, Punycode and plain ASCII fast paths

- Kind: perf
- Commit: 1e5f98fe6, 45cac086e, f0b7c8596, 277420e69
- Test: the corpus above, unchanged (arm64, LLVM, Go backend)
- Timing: host 1, every workload at or faster than x/net/idna (mixed 0.94, long 0.94, map 0.91, enc 0.98, dec 0.96, ascii 0.71): bench/results/std-twin-idna-2026-11-03.md, section "After the ASCII skip and rt.cast"

Runes below U+80 index their range directly (128 bytes), `validate_and_map` and `validate_label` decode UTF-8 in place (no rune list per label), Punycode writes into a BytesBuilder (a string per rune before), its output Str is not revalidated (`rt.cast`), and plain ASCII runes skip the table.
