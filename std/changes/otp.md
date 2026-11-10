# std/otp changes

## 2026-11-10: refactor: one file with its bodies (decision 0588 stage 2)

- Kind: refactor
- Commit: this commit
- Found by: decision 0588 stage 2 (pure modules merged with their implementation)
- Go reference: none (no behavior change)
- Test: internal/stdtest corpus otp on go, arm64 and llvm

std/otp.xo now holds the bodies of std/native/otp.xo (removed), and the Go
backend compiles them from it as its own package. The API, texts and errors
are unchanged: every item the declaration file had is `pub`, with the same
parameter names.

## 2026-11-03: perf: hotp with SHA-512 below Go's time

- Kind: perf
- Commit: c5fbea698 (Lang 1, SHA-512 instructions) and this push (SHA-2's finish)
- Found by: the std timing rows (std-twin-rows18: otp sha512 1.376 and sha256 1.327 of Go) and XO_PROF on linux/arm64
- Go reference: crypto/sha512 sha512block_arm64.s, crypto/sha256 checkSum
- Test: internal/stdtest corpora otp, crypto, aead and jwt on arm64 and Go; bench/perf/otp twincheck
- Timing: sha512 0.937 of Go's wall (was 1.376), sha256 1.316 (was 1.327) on linux/arm64 (bench/results/std-twin-otp-2026-11-03.md)
- Shipped in: not yet released

No change in std/otp itself: its codes come from crypto.hmac, whose
SHA-512 block now runs on arm64's SHA512 instructions and whose SHA-2
finish appends its padding at once (std/changes/crypto.md).

## 2026-11-03: perf: hotp follows crypto's faster HMAC and SHA-1

- Kind: perf
- Commit: 075763993, c5643cda2
- Found by: the std timing rows (std-twin-rows14: otp sha1 1.36 of Go) and XO_PROF on linux/arm64
- Go reference: crypto/hmac New (the pads), crypto/sha1 checkSum
- Test: internal/stdtest corpora otp, crypto and aead on arm64, LLVM and Go; bench/perf/otp twincheck
- Timing: otp sha1 1.071 of Go's wall (was 1.36), 0.969 of its CPU (bench/results/std-twin-rows18-2026-11-03.md)
- Shipped in: not yet released

No change in std/otp itself: its codes come from crypto.hmac, whose pads
and SHA-1 finish got faster (std/changes/crypto.md).

## 2026-11-03: feature: std/otp, HOTP and TOTP one time passwords

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 survey (std-coverage.md: TOTP, pquerna/otp); decision 0473 (Accepted, owner 2026-11-03)
- Go reference: RFC 4226 and RFC 6238 on crypto/hmac; pquerna/otp's Key.URL layout (reproduced with net/url, the module itself is not vendored)
- Test: internal/stdtest/testdata/corpus/otp (400 HOTP and 300 TOTP seeded cases over three algorithms and 6 to 8 digits, 80 URIs with awkward names, the RFC vectors, verify windows) on arm64, LLVM and the Go backend; bench/perf/otp twincheck
- Timing: bench/results/std-twin-rows11-2026-11-03.md (hotp: SHA-1 4.78, SHA-256 1.57, SHA-512 1.57 of Go's wall); after rt.sha1_block (b39aeaae1), bench/results/std-twin-rows14-2026-11-03.md: SHA-1 1.36, SHA-256 1.55, SHA-512 1.66; not Proven

`otp.hotp`, `totp`, `verify` (returns the matched step, `Int?`, so a caller can reject a replay), `generate_secret`, `uri` (spec/std.md 57).
