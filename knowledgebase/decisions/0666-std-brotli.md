# 0666: std/brotli, RFC 7932 Brotli encode and decode (a port of andybalholm/brotli)

Status: Proposed (xo-lang-3, 2026-11-15)

## Context

The owner asked for Brotli (task #67, through xo-4c on 2026-11-15). std/ui's wasm client module is 203 KB raw,
68.6 KB under gzip and 54.9 KB under `brotli -q 11` (bench/results/ui-wasm-client-2026-11-15.md, "Time to resume"):
about 70 ms less before prediction is ready on slow 4G. Every browser sends `br` in Accept-Encoding; Prometheus,
nginx and most CDNs speak it. Go's standard library has no Brotli, so std's rule (port Go's code where it fits) points
at the Go package the ecosystem uses: github.com/andybalholm/brotli v1.2.2 (MIT, a mechanical translation of Google's
C reference, which is MIT too), about 20,000 lines of logic plus RFC 7932's 122,784 byte static dictionary and the
encoder's lookup tables (about 1.1 MB of Go source as tables).

std/compress (gzip, deflate, zlib, lzw) is in xo's own build closure (`xo build --list-inputs ./compiler/xo`).

## Decision

1. A new module std/brotli, not part of std/compress: Brotli is its own format (RFC 7932, no deflate code in
   common), and keeping it out of std/compress keeps it out of what compiler/xo builds from, so the port's size
   never reaches the toolchain's build or a seed.
2. API, in std/compress's shape:
   ```
   fn encode(data: Bytes, quality: Int = 6, window: Int = 0) -> Bytes          // WriterOptions{Quality, LGWin}; 0 picks as Go does
   fn decode(data: Bytes, max_size: Int = 1073741824) -> Bytes ! BrotliErr   // NewReader + ReadAll, bounded
   struct BrotliErr { msg: Str }                                              // Go's texts ("brotli: ...")
   fn compressor(quality: Int = 6, window: Int = 0) -> Compressor              // write, flush, finish (Writer)
   fn decompressor(max_size: Int = 1073741824) -> Decompressor                // write, finish (Reader)
   fn responses(min_size: Int = 1024, quality: Int = 5) -> fn(http.Request, http.Next) -> http.Response
   ```
   `responses` is http.gzip_responses' twin: it answers `br` when the client's Accept-Encoding allows it and
   falls through otherwise, so `r.wrap(brotli.responses()).wrap(http.gzip_responses())` serves both. quality 6
   is andybalholm's DefaultCompression; a program compressing once (std/ui's client module, static assets) asks
   for 11. An out of range quality or window is a fault, as compress.Level is a closed set.
3. A port, not a reimplementation: andybalholm's encoder at every quality (0 and 1's fast paths, 2 to 9's hashers,
   10 and 11's Zopfli) and its decoder, with the attribution of decision 0302 (THIRD_PARTY_NOTICES [brotli]) and
   std/changes/brotli.md (0276). The output of `encode` is byte identical to andybalholm's at each quality and
   window, which the corpus checks; the static dictionary and lookup tables are generated .xo data
   (scripts/gen brotli, from the Go module, as std/publicsuffix's tables are).
4. Verification: a scripts/stdcorpus `brotli` corpus against andybalholm both ways (Xo's output decoded by Go and
   Go's output decoded by Xo, on text, binary, empty, incompressible and dictionary heavy inputs, every quality,
   several windows), the RFC 7932 and Google test vectors andybalholm's tests use, malformed and truncated
   streams with Go's error texts, a fuzz harness on decode, and timing rows against andybalholm (decode and
   encode at qualities 1, 6, 11) beside the corpus.
5. Order: decode first (smallest, and enough to read `br` from servers), then encode by quality band (0 to 1, 2
   to 9, 10 to 11), each its own push; then `responses` and std/ui's `br` branch for its client module (xo-4c
   offered that part).

## Consequences

- std/brotli is one of the larger std modules (its tables about the size of std/publicsuffix's); programs that
  never `use std/brotli` carry none of it.
- Byte identical output makes the corpus exact and lets a later speedup prove itself against fixed bytes.
- A push of std/brotli touches no closure file, so it gates on its own tests (Std Proven push rule).

## Alternatives

- Brotli inside std/compress (a `Format.Brotli` and `compress.brotli`): one place for every codec, but it puts a
  1 MB port into xo's build closure and every seed. A thin re-export can be added later if wanted.
- Decode only: enough to read compressed responses, not to serve them; the std/ui payoff needs the encoder.
- Calling the C library through `use c`: fast, but needs libbrotli on every host and target (no wasm, no static
  cross builds), against std's pure Xo rule.
- A fresh implementation from RFC 7932: a decoder is small, but a competitive encoder is the bulk of the work and
  would not match any reference's bytes.
