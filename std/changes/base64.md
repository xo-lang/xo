# std/base64 changes

## 2026-10-26: perf: the standard alphabet runs Bytes.to_base64 and from_base64, which decode eight characters at a time

- Kind: perf
- Commit: 25eed1215
- Found by: the std timing audit against Go's packages (bench/results/std-go-timing-audit-2026-10-26.md, bench/perf/base64)
- Go reference: encoding/base64 StdEncoding.EncodeToString, DecodeString and Decode's 8 byte fast path (go1.26.1)
- Test: internal/stdtest/testdata/corpus/base64 (go, arm64, LLVM); bench/perf/base64 twin (twincheck)

`base64.encode` with `Std` is `Bytes.to_base64` (a word at a time: 24 bytes in, 32 characters out) and
`base64.decode` with `Std` tries `Bytes.from_base64` first, which now reads eight characters with one word
load and eight table lookups and writes six bytes (32 characters and four words at a time through
`rs_push_words`), as Go's assemble64 does; the byte loop of this file stays for the other alphabets and,
on an error, finds the same error offset as before. Results and error offsets are unchanged (the corpus
agrees on three backends). `xo bench --ab` against Go's package, quiet, 20 rounds, 2,000 buffers of 0 to
3,000 bytes, 20 passes (setup alone: 0.55 of Go's wall), Xo / Go wall: encode 1.084 to 0.691,
decode 1.380 to 1.078 (instructions 1.97 to 1.31 of Go's).

## 2026-10-21: perf: decode through a table, four symbols at a time

- Kind: perf
- Commit: a021bbfb
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
- Commit: bdada12f
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
