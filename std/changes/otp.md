# std/otp changes

## 2026-11-03: feature: std/otp, HOTP and TOTP one time passwords

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 survey (std-coverage.md: TOTP, pquerna/otp); decision 0473 (Proposed)
- Go reference: RFC 4226 and RFC 6238 on crypto/hmac; pquerna/otp's Key.URL layout (reproduced with net/url, the module itself is not vendored)
- Test: internal/stdtest/testdata/corpus/otp (400 HOTP and 300 TOTP seeded cases over three algorithms and 6 to 8 digits, 80 URIs with awkward names, the RFC vectors, verify windows) on arm64, LLVM and the Go backend; bench/perf/otp twincheck
- Timing: see bench/results/std-twin-rows9-2026-11-03.md

`otp.hotp`, `totp`, `verify` (returns the matched step, `Int?`, so a caller can reject a replay), `generate_secret`, `uri` (spec/std.md 57).
