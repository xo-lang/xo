# std/idna changes

## 2026-11-14: perf: the rune table's block index covers the whole BMP

- Kind: perf
- Commit: this push
- Found by: the stdproven perf run for std/idna (linux/arm64 host 1: dec 1.060 of Go's wall, the only case over 1.05) and an XO_PROF profile of dec (lookup about 24%)
- Go reference: golang.org/x/net/idna reads its trie, a constant number of steps a rune
- Before: lookup's block index (BLOCKS) stopped at U+3400, so a rune above it (CJK, Hangul, the compatibility forms) binary searched all 7,380 ranges, several times a rune (mapping, validation, Bidi)
- Fix: BLOCKS covers every 64 rune block to U+10000 (2 KB more); scripts/genidna writes it, and std/idna.xo regains its END GENERATED marker, which the one-file merge (decision 0597) dropped, so genidna runs again. The other tables are unchanged. bench/perf/idna dec on linux/arm64 host 1, --ab against 7c0857749: 0.948 (0.92 to 0.975, n=6; 14.06 to 13.34 ms).
- Test: xo test --std-module=idna; TestStdCorpora idna and norm on go, arm64, llvm; TestIdnaFuzz
- Shipped in: not yet released

## 2026-11-14: perf, security: punycode encodes a long label in n log n

- Kind: perf, security
- Commit: this push
- Found by: the decision 0616 std/idna review (Xo Lang 3): RFC 3492's encode scans the label once per distinct non-ASCII code point, so a label of n distinct runes cost n squared steps; the Lookup profile (to_ascii's default, std/httpproxy's) has no length limit
- Go reference: golang.org/x/net/idna punycode.go encode, the same quadratic loop (64,000 distinct runes: x/net v0.60.0 20.4 s on a Mac); no vulndb entry
- Change: a label over 64 runes encodes through encode_long, which takes the runes in order of code point then position and counts the runes below the current code point between two emissions with a Fenwick tree over positions: the same deltas, output and int32 overflow errors in n log n. Shorter labels keep RFC 3492's loop
- Measured (Mac, native, distinct CJK runes): 16,000 runes 260 ms to 1 ms; 64,000 runes 1.27 s to 6 ms; bench/perf/idna (short names) on linux/arm64 host 1, --ab against the stream-safe norm commit, 11 rounds: enc 1.005 (0.989 to 1.028), mixed 1.003 (0.988 to 1.019), long 1.01 (0.999 to 1.05), within noise
- Test: std/idna_test.xo (x/net's answers for 6 labels of 65 to 464 runes, the 64,000 rune label, and decode's 1024 rune cap at and past it, as x/net); TestIdnaFuzz on arm64, llvm and go; TestStdCorpora idna on go, arm64, llvm
- Affected: every backend; bounded by what a caller passes (Registration refuses a label over 63 bytes, after encoding it)
- Shipped in: not yet released

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
