# 0305: std/compress (gzip, flate, zlib), streaming and bounded, and gzip responses

Status: Accepted (owner, 2026-10-21, within decision 0276)

## Context

std/archive (decisions 0164, 0172) already holds Go's compress/flate in
Xo. That covers the decompressor and the compressor at levels 5 and 6,
plus gzip headers and CRC-32, all over whole buffers. Three gaps remain:

- `gunzip` has no limit on its output, so a 1 KB bomb can expand to
  gigabytes.
- There is no zlib, no other compression level, and no streaming.
- Nothing compresses HTTP responses.

The coverage table (decision 0276) lists compress/gzip as done in memory
only and compress/zlib as planned. The design was drafted on 2026-10-21
(the lib1 note compress.md) and approved by the owner the same day.

## Decision

A module `std/compress` with Go's three formats, in memory and
streaming. std/archive keeps `gzip` and `gunzip` and calls into it.

```
enum Level { NoCompression, BestSpeed, Default, BestCompression, HuffmanOnly, Of(n: Int) }
struct CompressErr { msg: Str }

pub fn gzip(data: Bytes, level: Level = Level.Default) -> Bytes
pub fn gunzip(data: Bytes, max_size: Int = 1 << 30) -> Bytes ! CompressErr
pub fn deflate(data: Bytes, level: Level = Level.Default) -> Bytes
pub fn inflate(data: Bytes, max_size: Int = 1 << 30) -> Bytes ! CompressErr
pub fn zlib(data: Bytes, level: Level = Level.Default) -> Bytes
pub fn unzlib(data: Bytes, max_size: Int = 1 << 30) -> Bytes ! CompressErr

enum Format { Gzip, Deflate, Zlib }
pub fn compressor(format: Format, level: Level = Level.Default) -> Compressor
fn (var c: Compressor) write(data: Bytes) -> Bytes
fn (var c: Compressor) flush() -> Bytes
fn (var c: Compressor) finish() -> Bytes
pub fn decompressor(format: Format, max_size: Int = 1 << 30) -> Decompressor
fn (var d: Decompressor) write(data: Bytes) -> Bytes ! CompressErr
fn (var d: Decompressor) finish() -> () ! CompressErr
```

- **Go's bytes and texts.** Every level writes exactly what Go's
  compress/flate, gzip and zlib writers write, so the Go backend's output
  and the native output can be compared byte for byte. The errors are
  Go's texts.
- **Push style.** Bytes go in and bytes come out, with no I/O of their
  own. Readers and writers can wrap these types once a streaming I/O
  design exists.
- **`max_size` is where it differs from Go.** Go's readers have no limit.
  Here the decoders stop with `<format>: output exceeds max_size (N bytes)`.
  The default is 1 GiB.
- **The gzip writer sets no name and no time.** The reader accepts every
  header field and concatenated members.
- **HTTP**, an Xo addition: `http.gzip_responses(min_size: Int = 1024,
  level: compress.Level = compress.Level.Default)` is a middleware with
  gzhttp's rules:
  - compress when Accept-Encoding allows gzip;
  - skip a response that already has a Content-Encoding, is under
    `min_size`, is a 206 or a Range response, or has a Content-Type that
    is already compressed;
  - add `Vary: Accept-Encoding`, drop Content-Length, and weaken a strong
    ETag;
  - stream through a Compressor, flushed on each `BodyWriter.flush`.
- **Tests.**
  - Go's compress tests as std module tests, plus a golden on the Go
    backend.
  - The compress/testdata corpus compressed at every level must give Go's
    bytes.
  - Random and truncated streams must give Go's output or Go's error
    (scripts/stdcorpus).
  - A bomb must stop at `max_size`.
  - An HTTP golden on arm64, LLVM and Go.

## Consequences

- Implementation notes (2026-10-21):
  - `Decompressor.finish` returns the rest of the output, `Bytes !
    CompressErr`, not `()`. Go's gzip reader returns a member's last
    block only when it reads on past the member, so the end of a stream
    can carry output.
  - How much a `write` returns between flushes or the end may differ by
    backend; the concatenation does not.
  - Move only is a doc marker, `@move_only`, on std structs (core 5.3),
    read by the checker as it reads `@moves`.

- The Go backend wraps Go's compress packages (std/rt), and the native
  port is held to them.
- The levels and the stateful decompressor that are still missing are
  ported from Go: deflatefast.go, the config table with lazy matching,
  HuffmanOnly, and Flush with its sync blocks.
- Untrusted input gets a limit by default. A program that needs more
  passes `max_size`.

## Alternatives

- Keeping everything in std/archive and adding only zlib and the levels.
  Rejected: archive is about file containers, and Go splits these the
  same way (compress/* and archive/*).
- Readers and writers over a new byte source and sink interface now.
  Closer to Go's shape, but it would design streaming I/O as part of this.
- No limit, as in Go. Rejected: the decompression bomb is the reason this
  module exists natively, and an explicit limit is cheap.
