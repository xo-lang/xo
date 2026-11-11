# std/idna changes

## 2026-11-11: refactor: std/idna is one file (decisions 0588, 0597)

- Kind: refactor
- Commit: this push
- Found by: decision 0597 part 1 (rt.cast is a portable intrinsic)
- Go reference: none (no behavior change)
- Test: TestStdCorpora idna on go, arm64, llvm; TestStdChecks
- Shipped in: not yet released

std/native/idna.xo moved into std/idna.xo; std/rt/idna.go (IdnaErr's display on the Go backend) and the generated std_gen_idna.go and std_sigs_idna.go are gone: the Go backend compiles the module with the program.

## 2026-11-10: refactor: width bounds as the type constants (decision 0587 point 4)

- Kind: refactor
- Commit: this commit
- Found by: decision 0587 point 4 (std's literal bounds become the type constants)
- Go reference: none (no behavior change; Go's math.MaxInt32, MaxUint16, MaxUint32)
- Test: xo test --std-module=idna and its corpus where it has one

In std/idna, MAX_I32 is `Int(Int32.max)`. No behavior change.

## 2026-11-04: fix: a punycode label that decodes to a surrogate no longer faults

- Kind: fix (security: a denial of service)
- Commit: this push
- Found by: the owner's v0.38.1 evaluation (b27): `xn--ib9b` (U+D800) stopped a process, whatever the profile
- Go reference: x/net/idna (go1.26, v0.60.0): the decoded rune is converted with `string(rune)`, a surrogate becoming U+FFFD, which the Lookup, Display and Registration profiles refuse (`idna: invalid label "\ufffd"`) and the Punycode profile passes
- Test: golden std_surrogate_decoders (every backend, the Go outputs)
- Shipped in: not yet released

`idna.to_ascii` and `to_unicode` faulted (`Rune(55296)`) on a name with a label like `xn--ib9b`. The decoded surrogate is now U+FFFD as in Go. The error text also prints a printable non-ASCII rune as it is (Go's %q), where it escaped every one.

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
