# std/otp changes

## 2026-11-16: perf: a decimal code's modulus without a loop

- Kind: perf
- Commit: this push
- Found by: bench/perf/otp (#118); XO_PROF: pow10 4.5% of a code
- Go reference: github.com/pquerna/otp v1.5.0 hotp.GenerateCodeCustom (math.Pow10 of the digits)
- Test: the otp corpus (go, arm64, llvm); twincheck bench/perf/otp; xo bench --ab on linux/arm64 host 1 with the HMAC-SHA256 pads change: bench/perf/otp sha256 0.897
- Shipped in: not yet released
- Timing: bench/perf/otp (std/evidence/otp/perf.md)

`hotp` computed 10 to the power of its 6 to 8 digits with a loop on each
code; it now picks the constant.

## 2026-11-15: fix: uri escapes as pquerna/otp does; feature: hotp_uri

- Kind: fix, feature
- Commit: this push
- Found by: the stdaudit gaps of pquerna/otp (hotp.Generate); porting pquerna's TestGenerate, whose `issuer=Snake%20Oil` uri did not write
- Go reference: github.com/pquerna/otp v1.5.0 totp.Generate, hotp.Generate, internal.EncodeQuery
- Test: internal/stdtest/testdata/corpus/otp (scripts/stdcorpus otp): the 80 totp URIs now built as pquerna builds them (42 failed before the fix) and 40 hotp URIs (go, arm64, llvm)
- Shipped in: not yet released
- Timing: none

`uri` escaped its parameters as a query (`issuer=Ex+ample`) and each
label part as a path segment (`a%2Fb`). pquerna/otp escapes the values
with url.PathEscape (`issuer=Ex%20ample`) and the label as the URL's
path (a `/` stays). `uri` now writes pquerna's bytes; apps read both.
`hotp_uri` writes hotp.Generate's `otpauth://hotp/` URI (algorithm,
digits, issuer, secret; no counter).

## 2026-11-15: feature: Key.image, a QR code of the URI

- Kind: feature
- Commit: this push
- Found by: the stdaudit features line (decision 0655): otp.Key.Image
- Go reference: github.com/pquerna/otp v1.5.0 Key.Image over github.com/boombuler/barcode v1.0.1-0.20190219062509-6c824513bacc (qr.Encode at level M in Auto mode, barcode.Scale; MIT)
- Test: internal/stdtest/testdata/corpus/otp (scripts/stdcorpus otp): 296 cases (key URIs, numeric and alphanumeric contents, long URIs to version 40 and past it, 8 sizes) against Key.Image run by internal/stdtest/qrref, compared by the SHA-256 of the modules (go, arm64, llvm)
- Shipped in: not yet released
- Timing: none

`Key.image(width, height)` returns a Gray `png.Image` (0 dark, 255
light) with boombuler's modules, mask choice and scaling, and its error
texts. The penalty rules are boombuler's, not the QR standard's (rule 1
adds the run length minus 2), so the mask can differ from another
encoder's for the same text; the code reads the same.

## 2026-11-15: feature: parse_uri, Key and the Steam Guard encoder (pquerna/otp)

- Kind: feature
- Commit: this push
- Found by: the stdaudit features line (decision 0655): otp.NewKeyFromURL, Encoder, EncoderDefault, EncoderSteam, Key.Encoder
- Go reference: github.com/pquerna/otp v1.5.0 otp.go and hotp/hotp.go (Apache-2.0)
- Test: internal/stdtest/testdata/corpus/otp (scripts/stdcorpus otp): 120 Steam codes (1 to 8 letters, three algorithms) by pquerna's GenerateCodeCustom written out in Go, and verify of each; 22 URIs read by a Go mirror of pquerna's Key methods on net/url (missing, bad, signed and overflowing numbers, escapes, white space, steam); secret_bytes and its error (arm64, LLVM, Go)
- Timing: none

`otp.parse_uri(s)` gives a `Key` read as pquerna's Key methods read the
URI; `encoder: otp.Encoder.Steam` writes and checks Steam Guard codes.
Key.Image (a QR code) is still a gap. hotp still refuses Md5: its 16 byte
MAC is shorter than RFC 4226's truncation reads (pquerna's index goes out
of range there).

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
