kind: review
module: compress
result: pass
commit: b91e3c451
source: c6e28c6b292bd1f66102119f96c9c8a5f26d9474c9ea128482417979e5dba89d
date: 2026-11-14
host: darwin/arm64 (probes, goldens and timings)
command: none (section 3 does not apply: no secret, no timing test); read against knowledgebase/infrastructure/std-security-checklist.md
measure: 1 Go vulnerability entry for compress/* (GO-2022-0524, not shared: loops, tested at 10^6 members); 9 limits, 7 tested at and past them, 2 with no limit as Go (linear, tested); 5 defects found and fixed (F1 quadratic streaming decode of a long block, 2706 to 56 ms at 4 MB; F2 16 bytes of memory per output byte in a write, 3.34 GB to 420 MB at 206 MB; F3 output held back across writes; F4 input after a stream's end kept; G1 errors not sticky) and 1 bound documented with its std caller fixed (G2: one write's output bounded by max_size, not the write; std/http's client 6.4 GB to 23 MB on a 1000 MB bomb; ADR 0635 for 0.52); 2 fault sites, neither reachable from input; 0 insecure defaults (max_size 1 GiB by default, where Go has none)

Security review of std/compress (std/compress.xo, std/native/compress.xo,
std/native/llrt/flate.xo, and on the Go backend std/rt/compress.go over
Go's own compress/gzip, compress/flate, compress/zlib and compress/lzw)
against knowledgebase/infrastructure/std-security-checklist.md (decision
0616), by Xo Lang 2. std/compress decodes untrusted input (HTTP bodies with
Content-Encoding, uploads, archives through std/archive, PNG data), so it
opts in. Line numbers are at the commit above.

The decoders give Go's bytes or Go's error text (decision 0305), which the
corpora (internal/stdtest/testdata/corpus/compress, compressfuzz, lzw,
lzwfuzz), TestCompressFuzz and TestLzwFuzz check against Go on every native
backend. Two differences are by design and documented (std/compress.xo
Decompressor, spec/std.md): how much each Decompressor write returns
differs by backend, and a write that fails returns only the error, so
output it decoded before the error in that same write is not returned (Go's
Read returns those bytes, then the error on the next Read).

## 1. Go's CVE history

The Go vulnerability database (vuln.go.dev, index downloaded 2026-11-14:
all 188 stdlib entries fetched; the 6 a first pass missed, GO-2023-2185
path/filepath and GO-2026-6603, -6610, -6611, -6612, -6617 net/http, on a
retry) has one entry whose package is compress/gzip, compress/flate,
compress/zlib or compress/lzw:

- GO-2022-0524 (CVE-2022-30631): compress/gzip's Reader.Read recursed
  once per concatenated member, so many empty members exhausted the stack.
  Not shared: gunzip walks the members in a loop (flate.xo:2585
  gunzip_members), and the Decompressor's InStream.push loops through its
  states with no call per member. Test:
  internal/llvm/testdata/golden/compress_limits: 1,000,000 empty members
  then one of 3 bytes (23 MB) through gunzip and a Decompressor, on the main
  task and in a spawned task (a smaller stack), on arm64, LLVM, wasm and
  the Go backend (Go parity).

The Go backend runs Go's packages, so it carries Go's fix for the toolchain
it is built with (Go 1.26.11 since 0.51.0).

Sibling classes in Go's other decoders, checked:

- Memory from a claimed length or a ratio (GO-2022-1037, GO-2025-4014,
  GO-2026-4869 archive/tar): std/compress allocates nothing from a length
  the input claims (a stored block's LEN is copied as the input holds it,
  flate.xo data_block and stored_part; FEXTRA's length is skipped over the
  input), and output is bounded by max_size (section 2). tar and zip are
  std/archive's, reviewed on their own.
- Excessive CPU (GO-2026-4342 archive/zip): shared in kind, F1 and F3
  below, fixed.
- Panics on crafted input (GO-2021-0067, -0240, -0264, GO-2022-0273
  archive/zip): section 4.

## 2. Limits on hostile input

| Input a peer controls | Limit | Where | Test at and past it |
|---|---|---|---|
| Output of gunzip, inflate, unzlib | max_size (default 1 GiB); decoding stops as the output passes max_size + 1, as Go's LimitReader(max_size + 1) | compress.xo:61 limit_of, :63 over; flate.xo:1694, 1757, 1786 (one shot), 1868, 1912, 1974 (streaming) | compress_limits: max_size = length passes and length - 1 fails for gzip, deflate, zlib, one shot and streamed; a 1027:1 stream (10 MB of zeros in 10,206 bytes) stopped at 1 MB; compressfuzz corpus and harness at length - 1 for every stream that decodes |
| Errors of a Decompressor | the first error is kept: every later write and finish returns it without decoding, and the decoder's buffers are dropped (G1) | compress.xo Decompressor.stop | compress_stream_edges: a corrupt deflate, a corrupt zlib and a gzip over max_size, then a fresh valid stream twice and finish |
| Output of a Decompressor | max_size over all writes (the stream's limit max_size + 1, and the bytes given out counted against max_size) | compress.xo:316 give; flate.xo InStream (f.limit, f.dropped) | compress_limits streamed rows; compress_stream_edges: max_size + 1 bytes with a bad and a good CRC-32, as one shot |
| Memory of one Decompressor write | about twice the output of that write (the decoder's list and one Bytes copy); the output of one write is bounded by max_size, not by the write's size (DEFLATE gives up to about 1032 bytes a byte: G2, spec/std.md); std/http's client feeds the decoder 4 KiB at a time (GZ_SLICE), so its steps stay near 4 MB; a write bounded by its caller is ADR 0635 (0.52) | flate.xo drain, list_range_bytes; std/native/http.xo GZ_SLICE | timing below: a write inflating 1.3 MB to 206 MB peaks at 420 MB (one shot 419 MB); http_gzip_hostile: read(64 KiB) and read(1 MiB) of about 1 MB of gzip that inflates to 1000 MB keep the heap under 50 MB (23 MB RSS; 0.51.0 6.4 GB) |
| Output of lzw_decode | max_size | compress.xo:751 | compress_limits lzw row; TestLzwFuzz at length - 1 |
| gzip FNAME and FCOMMENT | 511 bytes and the NUL (Go's 512 byte buffer) | flate.xo:2531 | compress_stream_hostile: FNAME 511, 512, 1 MB; compress_stream_edges: FCOMMENT 511, 512; one shot and streamed |
| gzip FEXTRA | 65,535 bytes, skipped, not stored | flate.xo:2518 | compress_stream_hostile: 65,535 bytes in 64 byte writes and cut short |
| Dynamic Huffman header | HLIT at most 286 codes, HDIST at most 30, complete codes | flate.xo:1604, 1609, 1328 | compress_stream_edges: HLIT 29 (286) passes the check, 30 and 31 refused; HDIST 29 (30) passes, 30 and 31 refused; compressfuzz (random and bit-changed headers) |
| LZW codes | 12 bit, 4096 entries; a code above the table is `lzw: invalid code` | compress.xo:689 | lzw and lzwfuzz corpora, TestLzwFuzz |
| Concatenated gzip members; one DEFLATE block's length | none, as Go: linear time, no stack per member | flate.xo:2585, InStream huffman_part, stored_part | compress_limits (10^6 members); compress_stream_hostile (a 1 MB block in 64 byte writes); timings below |

Timings (darwin/arm64, 2026-11-14; probe programs as in the goldens):
one fixed Huffman block fed to a Decompressor in 16 KB writes, at 1, 2 and
4 MB: 178, 689, 2706 ms before F1, 14, 29, 56 ms after (7 byte writes 26,
54, 106 ms; one shot inflate 5, 10, 20 ms). The goldens check the outputs;
a quadratic decoder would also time them out (the 1 MB block in 64 byte
writes is about 16,000 writes).

### Defects found and fixed

- F1 (42dec4ba7): the native Decompressor decoded a block whose input had
  not all arrived again from its start on every write, and copied all the
  waiting input each time: one block of n bytes in k byte writes cost
  n * n / k (a hostile 64 MB block about 11 minutes of CPU). A block in
  progress now goes on from its last whole symbol: flate.xo huff_one reads
  all of a symbol's bits before it writes output, so going back to the end
  of the last whole symbol repeats nothing; a stored block goes on from its
  next byte.
- F2 (0328dd209): each write's output went through a List[Int] (8 bytes a
  byte) twice: about 16 bytes of memory per output byte, so one 5 MB write
  could reach 17 GB under the default max_size. Now copied once into
  Bytes: 3.34 GB to 420 MB for 206 MB of output.
- F3 (0328dd209, a regression of 42dec4ba7): a header cut short waited
  until the waiting input doubled, which held back a complete stream until
  `finish` (a gzip member cut in its name). A short gzip header now waits
  for the bytes it still needs (flate.xo:2564 gzip_need: a whole FEXTRA
  field, else one more byte, names being at most 511); a dynamic header,
  at most about 700 bytes, is reread on every write. Test:
  std/native/llrt/flate_test.xo (output on the push that completes the
  header, and a byte at a time).
- G1 (822b84ef5, Lang 3's second read): after an error the native
  Decompressor parsed whatever later writes gave it, so the error changed
  between calls and only the decoder's state kept a later write from
  succeeding (none was seen to). It now keeps its first error, returns it
  from every later write and finish without decoding, and drops its
  buffers, as Go's readers and the Go backend do.
- G2 (Lang 3's second read; std/http bb3b79dce): one write's output is
  bounded by max_size, not by the write (a 260 KB gzip of 256 MB of zeros
  in 32 KB writes gave up to 35.9 MB per write). std/http's client, the
  std caller that streams untrusted bodies, now feeds 4 KiB at a time; the
  bound is in spec/std.md; a write bounded by its caller is ADR 0635.
- F4 (0328dd209): input written after a zlib or raw stream's end was joined
  to the buffer and kept; it is now dropped (flate.xo:2734), as Go's
  readers stop at the end. Tests: flate_test.xo (1 MB after the end, the
  buffer stays under 1 KB), compress_stream_edges.

## 3. Constant time

Not applicable: std/compress handles no secret. (Compressing a secret next
to attacker-chosen data leaks through the output's length, CRIME and
BREACH; that is the caller's protocol choice, as with Go's packages.)

## 4. Fault behaviour

Fault sites:

- compress.xo:20 level_of: `Level.Of(n)` outside -2 to 9 faults, as
  flate.NewWriter returns an error. The level is the caller's argument,
  never peer input; std/compress.xo's Level doc says so.
- flate.xo load32 (fault on a read past the compressor's buffer): an
  invariant of the BestSpeed encoder, reached only from compressing.

Every read of input on the decode paths is guarded: byte_of, get_u16 and
get_u32 run only after a length check (gzip_header, data_block,
block_head, the gzip and zlib trailers, unzlib's header); the bit reader
(more_bits, huff_sym) returns unexpected EOF at the end of the input;
Huffman tables are built only from complete codes (flate.xo:1328) and
indexed by at most 9 bits and the link mask; huff_sym reports a zero
length code as corrupt; read_huffman refuses a
repeat at index 0 and a run past the lengths; the LZW decoder rejects a
code above its table (compress.xo:689), its prefix chains strictly
decrease, and its 8192 byte buffer (compress.xo:588) holds a batch's 4096
bytes plus the longest chain (Go's 2 * maxCode).

Unchecked memory operations on the decode path, and the invariant each
relies on:

- raw.u8s_copy_back (flate.xo:1756 one shot, 1973 streaming): copies
  `length` bytes from `dist` back in out; guarded by dist <= hist, where
  hist = min(out.len() - out_start, WINDOW_SIZE), checked just before; the
  streaming drain keeps the last WINDOW_SIZE bytes and clamps out_start
  (flate.xo:2699), so out.len() - out_start never counts dropped bytes.
- rt.mem_copy in list_bytes and list_range_bytes (flate.xo:1394, 1414):
  copies n bytes from a List[UInt8] of at least n elements (from <= to <=
  out.len(), the callers' arguments).
- mem_load in the compressor (flate.xo:1043, 1114, 1149, 1155, 1164,
  2084, 2130): reads within the window or the input, which the encoder's
  match limits keep inside; compress side only.

No `!` unwraps or or_fault in these files. The fuzzing backs this:
TestCompressFuzz and TestLzwFuzz build hostile streams bit by bit, and
their long runs (fuzz.md) found no fault.

## 5. Defaults

- max_size defaults to 1 GiB on every decoder, a bound Go's readers do not
  have (Go's callers add io.LimitReader). The largest DEFLATE ratio is
  about 1032:1, so 1 MB of input can still ask for 1 GiB of output; a
  server decoding uploads should pass a smaller max_size. std/http's server
  does not decode request bodies; its client decodes a gzip response
  through a Decompressor with the default, 4 KiB of input at a time
  (GZ_SLICE), and ClientStream.read_all gathers up to its own max_size
  (default 1 GiB), as Go's io.ReadAll gathers all: a whole-body read of a
  1000 MB bomb peaks at 1.1 GB (read_all) or 2.1 GB (send), where Go's
  http.Get and io.ReadAll peak at 2.1 GB.
- No insecure option exists: no setting weakens a decoder, and the
  decoders accept only what Go's accept (zlib's preset dictionary only as
  the empty one).

## Sign-off

First read (an independent subagent of Xo Lang 2, 2026-11-14, at
25665ee40): findings 1 (output held back: F3), 2 (memory per write: F2),
3 (input after the end kept: F4), all fixed in 0328dd209 with tests; 4
confirmed F1's resume logic and sections 1 and 4; 5 to 7 (raw memory
sites, file:line, placeholders, FCOMMENT and HLIT/HDIST tests, timings,
the measure count) and 8 (output lost on a failing write; the max_size +
1 order) answered in this text and compress_stream_edges.

Second reader: Xo Lang 3, 2026-11-14, read at df1d65cd7, then 822b84ef5 (with main's bb3b79dce for std/http). Probed on arm64, llvm and go against the sections' claims. Findings: G1, native Decompressor errors were not sticky (later writes decoded on and the error changed; no Ok after an error was seen); fixed in 822b84ef5, which keeps the first error, drops the decoder's state, and is tested in compress_stream_edges (a corrupt deflate, a corrupt zlib, gzip over max_size, then fresh valid streams). G2, one write's output is bounded by max_size, not by the write (a 260 KB gzip of 256 MB of zeros in 32 KB writes returned up to 35.9 MB per write, 360 MB peak); std/http's client feeds the decoder GZ_SLICE (4 KiB) at a time and http_gzip_hostile holds read(64 KiB) and read(1 MiB) of a 1000 MB bomb under 50 MB of heap; the per-write bound is documented in spec/std.md and sections 2 and 5, and a bounded write is planned for 0.52 (ADR 0635). I agree with sections 1, 3, 4 and 5 and with F1 to F4.
