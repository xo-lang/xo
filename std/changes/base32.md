# std/base32 changes

## 2026-10-26: perf: encode takes whole five byte blocks as one word

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 2's twin against encoding/base32 (bench/perf/base32, std-go-timing-audit)
- Go reference: encoding/base32's Encode loop
- Test: internal/stdtest/testdata/corpus/base32 (go, arm64, LLVM); twincheck bench/perf/base32 agrees on none, encode, decode
- Timing: bench/perf/base32 (2000 buffers of 0 to 3,000 bytes, 20 passes), quiet --ab, Xo / Go wall: encode 2.08 to 1.13, decode 0.85, none 0.57; encode against the parent 0.524

Each whole block is read into one 40 bit word and written as eight
symbols, with no per byte test of the block's length; the last, partial
block keeps the general loop.

## 2026-10-21: perf: decode through a table, eight symbols at a time

- Kind: perf
- Commit: 4706a035
- Found by: XO_PROF=cpu (the per quantum lists were 40% of the native decode)
- Go reference: encoding/base32 decode (go1.26.1)
- Test: internal/stdtest/testdata/corpus/base32 (go, arm64, LLVM); golden base64_basic

Decode looks each byte up in a 256 byte table, reads eight plain symbols at
once as one 40 bit integer, and keeps the slow quantum (padding, errors,
short ends) with its symbols in an integer instead of two lists. 1 KB encode
and decode, 20,000 times, median of 5: native 296 ms to 74 ms (Go's package
61 ms), the Go backend 421 ms to 227 ms. Results and error offsets are the
same.

## 2026-10-21: behavior: the Go backend runs base32.xo too

- Kind: behavior
- Commit: f1100ad6
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
