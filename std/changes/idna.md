# std/idna changes

## 2026-11-03: feature: std/idna, internationalised domain names

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 backend survey (std-coverage.md row 12: Unicode normalization (NFC) and IDNA); decision 0476 (std/idna Accepted, owner 2026-11-03)
- Go reference: golang.org/x/net v0.60.0 idna (idna.go, punycode.go) and golang.org/x/text v0.43.0 secure/bidirule, BSD-style license
- Test: internal/stdtest/testdata/corpus/idna (about 1,000 names on the four profiles, ToASCII and ToUnicode, with the error code, compared with x/net; 31 punycode vectors of x/net's punycode_test.go; known names) on arm64, LLVM and the Go backend; bench/perf/idna twincheck against x/net
- Cost and timing: bench/results/std-idna-size-2026-11-03.md (size beside Go's, verdict) and bench/results/std-twin-rows15-2026-11-03.md (timing)

`idna.to_ascii`, `idna.to_unicode`, `idna.Profile`, `idna.IdnaErr`, `idna.unicode_version` (spec/std.md 61). Plain ASCII names (letters, digits, hyphen, dot) skip the table.
