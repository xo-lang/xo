# std/base32 changes

## 2026-10-20: port: std/base32 from Go

- Kind: port
- Commit: (filled in after the push)
- Found by: survey
- Go reference: encoding/base32/base32.go (go1.26.1)
- Test: std/native/base32_test.xo; internal/llvm/testdata/golden/base64_basic

New module: Go's base32 alphabets (Std, Hex, RawStd, RawHex) with Go's decoding rules and error offsets.
