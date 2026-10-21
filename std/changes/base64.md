# std/base64 changes

## 2026-10-21: perf: decode through a table, four symbols at a time

- Kind: perf
- Commit: (filled in after the push)
- Found by: XO_PROF=cpu of the native decode (61% in decode, 25% in a per quantum list and its frees)
- Go reference: encoding/base64 decode and its fast path (go1.26.1)
- Test: internal/stdtest/testdata/corpus/base64 (go, arm64, LLVM); golden base64_basic

Decode looks each byte up in a 256 byte table (no branches per symbol),
reads four plain symbols at once as Go's fast path does, and keeps the slow
quantum (newlines, padding, errors) with the symbols in one integer instead
of a list. 1 KB encode and decode, 20,000 times, median of 5: native 169 ms
to 33 ms (Go's package 51 ms), the Go backend 591 ms to 172 ms. Results and
error offsets are the same.

## 2026-10-21: behavior: the Go backend runs base64.xo too

- Kind: behavior
- Commit: (filled in after the push)
- Found by: decision 0312 (std written once)
- Go reference: encoding/base64 (go1.26.1), as ported in std/native/base64.xo
- Test: internal/stdtest/testdata/corpus/base64 (go, arm64, LLVM); internal/llvm/testdata/golden/base64_basic (Go parity)

The Go backend compiles std/native/base64.xo instead of calling Go's
encoding/base64, so every backend runs one implementation; results are the
same. On the Go backend a 1 KB encode and decode takes 30 us instead of 2 us
(20,000 loops: 593 ms against 37 ms), the cost of gen's byte loops.

## 2026-10-20: port: std/base64 from Go

- Kind: port
- Commit: 4c5b0df2
- Found by: survey
- Go reference: encoding/base64/base64.go (go1.26.1)
- Test: std/native/base64_test.xo (3440 Go cases with base32); internal/llvm/testdata/golden/base64_basic

New module: Go's four base64 alphabets (Std, Url, RawStd, RawUrl) with Go's decoding rules and error offsets.
