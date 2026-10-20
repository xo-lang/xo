# std/base64 changes

## 2026-10-20: port: std/base64 from Go

- Kind: port
- Commit: 4c5b0df2
- Found by: survey
- Go reference: encoding/base64/base64.go (go1.26.1)
- Test: std/native/base64_test.xo (3440 Go cases with base32); internal/llvm/testdata/golden/base64_basic

New module: Go's four base64 alphabets (Std, Url, RawStd, RawUrl) with Go's decoding rules and error offsets.
