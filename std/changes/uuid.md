# std/uuid changes

## 2026-10-27: perf: the text is written and read through std/internal hooks with tables

- Kind: perf
- Commit: this commit
- Found by: the uuid twin's bytes row (7.1x google/uuid, the worst row of the std timing audit) and its CPU profile (decoding a digit at a time through a call was 54% of bytes and of parse)
- Go reference: google/uuid's encodeHex and xvalues table
- Test: internal/stdtest/testdata/corpus/uuid (go, arm64, LLVM); twincheck bench/perf/uuid agrees on none, parse, string, bytes; internal/stdcoverage allow lines for the two hooks (reached through uuid)
- Timing: bench/perf/uuid, quiet --ab, 15 rounds, Xo / Go wall: bytes 7.14 to 4.24 (0.606 of the parent), parse 1.53 to 1.23 (0.801), string 0.35, none 0.88

std/internal's uuid_text (16 bytes to the canonical text) and uuid_decode
(the text, dashed or 32 digits, to 16 bytes, None when malformed) are
stores and loads into one buffer with a digit table and a value table
(google/uuid's xvalues: one load a digit, one test after the loop) on the
native backends, and Go code on the Go backend (std/rt). parse, from_bytes,
v4, v7 and bytes() go through them. Still over Go: bytes allocates the 16
bytes, the Result and the text, where google/uuid's FromBytes copies into a
value array.

## 2026-10-26: perf: parse, bytes and from_bytes write and read the canonical text directly

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 2's twin against github.com/google/uuid (bench/perf/uuid, std-go-timing-audit)
- Go reference: google/uuid's Parse (xvalues by position) and encodeHex
- Test: internal/stdtest/testdata/corpus/uuid (go, arm64, LLVM); twincheck bench/perf/uuid agrees on none, parse, string, bytes
- Timing: bench/perf/uuid, quiet --ab, 15 rounds, Xo / Go wall: parse 2.79 to 1.53, bytes (bytes() and from_bytes) 9.66 to 7.14, string 0.35, none 0.90; against the parent: parse 0.541, bytes 0.739

parse writes the canonical text straight from the input's hex digits,
lowercased, by offset (it sliced, decoded 16 bytes, then formatted them
with to_hex and an interpolation, and built a list of positions per call);
from_bytes writes the 36 characters once; bytes() decodes the text in
place (it was a replace and from_hex). Still over Go: a Uuid holds its
text, so from_bytes formats where google/uuid copies 16 bytes, a write
call a character.

## 2026-10-21: behavior: the Go backend runs uuid.xo too

- Kind: behavior
- Commit: e8e2b322
- Found by: decision 0312 (std written once, the pilot's second module)
- Go reference: github.com/google/uuid v1.6.0, as ported in std/native/uuid.xo
- Test: internal/stdtest/testdata/corpus/uuid (go, arm64, LLVM); internal/llvm/testdata/golden/uuid_basic (Go parity)

The Go backend compiles std/native/uuid.xo instead of its own Go copy, so
every backend runs one implementation. The results are the same.

## 2026-10-20: port: std/uuid, RFC 9562 version 4 and 7 UUIDs

- Kind: port
- Commit: 662054a1
- Found by: survey
- Go reference: github.com/google/uuid v1.6.0 uuid.go, version4.go, version7.go (outside Go's std)
- Test: std/native/uuid_test.xo (google/uuid's Parse results on its test inputs and random UUIDs)

New module: `v4` and `v7` from the Rand and Clock capabilities, and
`parse` with google/uuid's accepted forms and error texts, on every
backend.
