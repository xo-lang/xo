# std/otp changes

## 2026-11-03: feature: std/otp, HOTP and TOTP one time passwords

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 survey (std-coverage.md: TOTP, pquerna/otp); decision 0473 (Accepted, owner 2026-11-03)
- Go reference: RFC 4226 and RFC 6238 on crypto/hmac; pquerna/otp's Key.URL layout (reproduced with net/url, the module itself is not vendored)
- Test: internal/stdtest/testdata/corpus/otp (400 HOTP and 300 TOTP seeded cases over three algorithms and 6 to 8 digits, 80 URIs with awkward names, the RFC vectors, verify windows) on arm64, LLVM and the Go backend; bench/perf/otp twincheck
- Timing: bench/results/std-twin-rows11-2026-11-03.md (hotp: SHA-1 4.78, SHA-256 1.57, SHA-512 1.57 of Go's wall: not Proven, the hash has no hardware SHA-1 path)

`otp.hotp`, `totp`, `verify` (returns the matched step, `Int?`, so a caller can reject a replay), `generate_secret`, `uri` (spec/std.md 57).
