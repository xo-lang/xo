# std/base32 changes

## 2026-10-21: behavior: the Go backend runs base32.xo too

- Kind: behavior
- Commit: (filled in after the push)
- Found by: decision 0312 (std written once)
- Go reference: encoding/base32 (go1.26.1), as ported in std/native/base32.xo
- Test: internal/stdtest/testdata/corpus/base32 (go, arm64, LLVM); internal/llvm/testdata/golden/base64_basic (Go parity)

The Go backend compiles std/native/base32.xo instead of calling Go's
encoding/base32, so every backend runs one implementation; results are the
same. On the Go backend a 1 KB encode and decode takes 27 us instead of 4 us
(20,000 loops: 538 ms against 86 ms), the cost of gen's byte loops.

## 2026-10-20: port: std/base32 from Go

- Kind: port
- Commit: 4c5b0df2
- Found by: survey
- Go reference: encoding/base32/base32.go (go1.26.1)
- Test: std/native/base32_test.xo; internal/llvm/testdata/golden/base64_basic

New module: Go's base32 alphabets (Std, Hex, RawStd, RawHex) with Go's decoding rules and error offsets.
