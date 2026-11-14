kind: review
module: png
result: pass
commit: 002149649
source: 20cae42cca83d9b01853bec040c435bc21710fe3543646373b24317a1bef938a
date: 2026-11-14
host: darwin/arm64 (reading, probes and Go 1.26.11 twins)
command: none (section 3 does not apply: no secret); read against knowledgebase/infrastructure/std-security-checklist.md
measure: 0 Go vulnerability entries for image/png (182 stdlib entries read, 2026-11-14; the 13 golang.org/x/image entries are tiff, webp, vp8l, bmp and sfnt, read as siblings); 2 inputs a peer controls (a file to decode or decode_config, an image to encode when a program encodes a peer's pixels), each bounded: max_pixels (100,000,000 by default; Go has none) before any allocation, the IDAT stream inflated to at most the size the header implies plus one byte, chunk lengths checked before every read, PLTE and tRNS at most 256 entries; 0 defects open (F1, decode with a max_pixels at or above about 2^60 faulted on integer overflow for an 8 byte pixel header where Go answers `dimension overflow`, fixed in 6988de4ee); 2 bounds and differences documented (G1: a valid image costs two to four times its pixels, 1.6 GB for a 10000x10000 16 bit RGBA image (Go 0.8 GB), 2.4 GB interlaced (Go 1.6 GB) and 3.2 GB for 100000000x1 at the default limit, where the row buffer is twice the image; G2: an over-long stream's error text differs from Go's, and decode refuses as dimension overflow the headers whose rows sum past 2^62, which Go's Decode passes and then panics on (makeslice) when an IDAT follows); fault sites: encode's and the Image helpers' caller contracts; 0 insecure defaults

Security review of std/png (std/png.xo, which every backend compiles,
decision 0312, over std/compress's zlib, std/crc32 and std/binary)
against knowledgebase/infrastructure/std-security-checklist.md
(decision 0616), by Xo Lang 2. std/png is a port of Go's image/png
(reader.go, writer.go, paeth.go of go1.26) over whole buffers (decision
0466): decode, decode_config and encode, the Image helpers image, at
and bytes_per_pixel (spec/std.md 43). It reads files a peer sends:
uploads, avatars, images fetched from a URL.

The probes of this review are an Xo program and two Go programs
outside the repository (std/png has no _test.xo):
~/xo-lead/notes/xo-lang-2/prov/review/pngprobe (main.xo CASE: big,
bigil, huge, bomb, chunks, idats, pal, flat, flat16, file PATH), gotwin
(Go's png.Decode of the same files) and gobomb (Go's answers on the
bomb, large header and huge header inputs), built with the xo of
thr0638 against this worktree's std (native backend) and Go 1.26.11 on
the Mac.

## 1. Go's CVE history

The Go vulnerability database (vuln.go.dev, index/modules.json and the
182 distinct stdlib entries, fetched on 2026-11-14) has no entry for
image/png or any image/* package of the standard library; the
compress/* entry (GO-2022-0524, gzip's stack exhaustion on many empty
members) is gzip's, which std/png does not use.

Sibling classes, from the golang.org/x/image entries (read 2026-11-14):

- Allocation from a declared size before the data is read
  (GO-2026-4815 tiff IFD offset, GO-2026-5062 tiff tile sizes,
  GO-2026-6222 vp8l, GO-2026-4962 sfnt, GO-2023-1572 tiff): decode
  refuses a header over max_pixels before anything is allocated
  (png.xo:572, `image too large`), and below it reserves pixels only as
  far as the inflated data reaches (read_pass, 749:
  `min(width * height * bytes, 32 * raw + 64)`; decode_rows, 1116, an
  interlaced image's full size only when the data covers it). A 113
  byte file declaring 10000x10000 gray fails in under 1 ms with 4 MB
  resident (Go: 0.2 ms; Go's lazily zeroed image costs it nothing).
- Decompression bombs (GO-2026-5032 tiff PackBits): the IDAT stream is
  inflated with a max_size of the size the header implies plus one
  (decode_image, 1065; expected_size, 707), so 64 MB of zeros in a 65
  KB file for a 100x100 image stops after 10,102 bytes, in under 1 ms
  (Go: under 0.1 ms; text differs, G2).
- CPU on degenerate sizes (GO-2023-1990 tiff 0-height): a width or
  height under 1 is `non-positive dimension` (parse_ihdr, 518-520), as
  Go.
- Panics on out of range palette indexes (GO-2024-2937 tiff,
  GO-2026-5031 bmp, GO-2026-5061 webp alpha): the decoder's palette is
  always 256 entries (dec_new, 1146-1152), an index past the PLTE extends
  the image's palette with opaque black (palette_of, 1017, as Go), and
  section 4 lists no reachable index fault.
- Overflow on 32-bit sizes (GO-2026-4961 webp on 32-bit platforms):
  Int is 64 bits on every Xo target; F1 was this class at 64 bits
  (fixed: Go's check and a row sum bound, section 4).

## 2. Limits on hostile input

| Input a peer controls | Limit | Where | Test at and past it |
|---|---|---|---|
| Width x height | max_pixels, 100,000,000 by default, checked before any allocation (`>`: exactly max_pixels passes); w and h read as int32 and refused at or below 0, as Go | parse_ihdr (489-579; the check 572) | pngprobe big: 10000x10000 gray with 3 rows at the default, `not enough pixel data` in 0 ms, 4 MB (Go the same error); the same file at max_pixels 99,999,999 and 10001x10000 at the default, `image too large`; bigil: 10000x10000 16 bit RGBA interlaced, `not enough pixel data` in 0 ms, 4 MB; huge: `dimension overflow`, as Go (F1, fixed); corpus png "a header too large for an Int is refused, not overflowed" |
| IDAT decompressed size (the ratio) | the size the header implies plus one byte (unzlib's max_size, so inflating stops there); more is `too much pixel data` or the zlib max_size error (G2) | decode_image (1063-1097), partial_inflate (1027-1057) | pngprobe bomb: 64 MB of zeros (65 KB file) for 100x100, refused in 0 ms, 11 MB resident, interlaced the same; a stream of exactly the image's size decodes (corpus png and pngfuzz) |
| Chunk lengths | an unknown chunk over 2^31 - 1 is `Bad chunk length` (Go's text); every length is checked against the bytes there before any read (`unexpected EOF`); IHDR exactly 13, IEND 0 | walk (1299-1304), check_crc (480-487), parse_ihdr (490) | pngprobe chunks: length 2^31, `Bad chunk length: 2147483648`; 2^31 - 1 with 3 bytes there, `unexpected EOF` |
| Chunk count | none (Go: none); each chunk costs its 12 bytes and a CRC over its data, so linear | walk (1168-1317) | pngprobe chunks: 1,000,000 empty tEXt chunks (12 MB) in 37 ms; idats: 1,000,001 IDAT chunks, 1,000,000 empty, in 30 ms, 45 MB resident |
| PLTE entries | length a multiple of 3, 1 to 256 entries and at most 2^depth, as Go (`bad PLTE length`) | parse_plte (581-603) | pngprobe pal: 1, 255 and 256 entries decode, 257 refused; at 4 bits 16 decode, 17 refused |
| tRNS length | gray 2, truecolor 6, paletted at most 256 (`bad tRNS length`); a paletted tRNS only after PLTE (`chunk out of order`) | parse_trns (605-652), walk (1198-1211) | pngprobe pal: paletted 0, 4 and 256 decode, 257 refused; truecolor 5 and 7 refused, 6 decodes; tRNS before PLTE refused |
| Interlace passes | 7 fixed passes; each pass's rows are checked against the data before they are read; the full image is allocated only when the data covers every pass | decode_rows (1099-1144), read_pass (732-825) | pngprobe bigil above; corpus pngfuzz (interlaced files cut and changed) |
| Rows and filter bytes | each row's bytes checked before it is read (`not enough pixel data`), a filter byte over 4 is `bad filter type` | read_pass (741-813) | fuzz TestPngFuzz; corpus pngfuzz |
| An image a program encodes from a peer's pixels | width and height 1 to 2^32 - 1, a palette of 1 to 256 (Format errors); pix of the size the header says (a fault, caller contract, section 4) | encode (192-321) | corpus png (encode); stress std/evidence/png/stress |

## 3. Constant time

Not applicable: std/png handles no secret.

## 4. Fault behaviour

### F1 (fixed in 6988de4ee): decode with a max_pixels at or above about 2^60 faulted on Int overflow

parse_ihdr computes `w * h` (at most (2^31 - 1)^2, under 2^62, no
overflow) and refuses it over max_pixels. Below that, expected_size
(710: `d.h * (1 + (bpp * d.w + 7) / 8)`), read_pass (749:
`width * height * mbytes`) and decode_rows (1117-1119:
`d.w * d.h * mbytes`) multiplied by up to 8 bytes a pixel, which
overflowed Int, a fault, once w * h passed about 2^60 (at 97fc633a8).
At the default max_pixels this cannot happen; a program that passed a
larger one, such as `Int` max to mean "no limit" (Go's behaviour),
faulted on a file of under 100 bytes:

```
IHDR 2147483647 x 2147483647, depth 16, color type 6 (RGBA), any IDAT
png.decode(f)                                   -> image too large
png.decode(f, max_pixels: 1 << 60)              -> image too large
png.decode(f, max_pixels: 9223372036854775807)  -> fault: integer overflow:
    2147483647 * 17179869177 does not fit Int at std/png.xo:706:12 (97fc633a8)
Go's png.Decode and png.DecodeConfig           -> png: unsupported feature: dimension overflow
```

Go's parseIHDR refuses a header whose pixel count times 8 overflows
int (`dimension overflow`, an UnsupportedError), which std/png did not
port, relying on max_pixels. Reproducer:
~/xo-lead/notes/xo-lang-2/prov/review/pngprobe (`main huge`), and
gobomb for Go's answer. Every backend (the arithmetic is png.xo's own),
since std/png's decoder (decision 0466).

Fixed in 6988de4ee (std/changes/png.md, CHANGELOG 002149649), as the
second reader's bound asked. parse_ihdr (522-524) ports Go's check
where Go makes it, after the non-positive test and before the bit
depth and color type: `w * h > (2^63 - 1) / 8` is
`unsupported("dimension overflow")` for decode and decode_config alike,
so decode_config now answers Go's on those headers (G2's second bullet).
walk (1186-1189), for decode only, also refuses a header whose packed
row of `1 + (bpp * w + 7) / 8` bytes times h passes `(2^63 - 1) / 2`
with the same error, which Go's check lets through ((2^30 + 1) x
(2^30 - 1) RGBA16; Go's Decode then fails at the missing data for a
header only file, and panics with `makeslice: len out of range` when an
IDAT follows, where Xo now answers an error). Under the row bound
expected_size's sum (h times the row) stays under 2^62. read_pass's and
decode_rows' `w * h * mbytes` are not bounded by the row sum (16 bit
gray, truecolor and gray alpha with tRNS decode to an 8 byte model from
2 to 6 packed bytes): they are held by Go's check at 522 with
model_bytes at most 8, so at most 8 (2^60 - 1) = 2^63 - 8. The interlaced sum at 716 adds at
most 11h/4 + 14 rows' filter and rounding bytes (each pass row at most
1 + bpp * w_p / 8 + 1, the seven passes' rows about 15h/8), under 2^34.
decode_config keeps Go's answer (a size and model) for those headers.
Test: corpus png "a header too large for an Int is refused, not
overflowed": nine header only files at max_pixels Int max (the 2^31 - 1
square at depth 16, 8 and an invalid 7, (2^30 + 1) x (2^30 - 1) at
RGBA16 plain and interlaced, RGB8, gray8 and interlaced gray1, and
(2^31 - 1) x (2^29 - 1) gray8), decode_config against Go's
DecodeConfig exactly and decode against Go's Decode or `dimension
overflow` where the row bound refuses, on arm64, LLVM and the Go
backend; before the fix the first file faulted.

### G1 (bound documented): a valid image costs two to four times its pixels, up to max_pixels

decode inflates the whole stream (a filter byte and the packed pixels
of every row) and then converts it row by row into the image's pix, so
both are held at once; read_pass also holds the current and previous
unfiltered rows (`buf`, 2n bytes for a packed row of n, 742-745),
which is twice the image when it is one row high; an interlaced image
also holds each pass's pixels before merging them into the full
image. A small file can
declare a large valid image (a flat 10000x10000 16 bit RGBA image is
760 KB). Measured decoding files from disk (pngprobe file, gotwin, the
Mac, native):

| Image (file size) | std/png | Go 1.26.11 |
|---|---|---|
| 10000x10000 gray, flat (95 KB) | 175 ms, 204 MB | 47 ms, 107 MB |
| 10000x10000 16 bit RGBA, flat (760 KB) | 1.23 s, 1.6 GB | 0.55 s, 808 MB |
| the same, interlaced (760 KB) | 2.10 s, 2.4 GB | 0.82 s, 1.6 GB |
| 100000000x1 16 bit RGBA, flat (778 KB; second reader's probe reader2/pngwide) | 1.81 s, 3.2 GB | not run (about 2.4 GB by the same terms: Go holds the two rows and the image, not the stream) |

So memory up to 2.0 times Go's (1.5 interlaced) and time 2.2 to 3.7
times on these flat images (a perf item: Go reads rows as the stream
inflates, so it never holds the inflated stream). Bounded by
max_pixels: the largest a decode can hold at the default 100,000,000
pixels is about 4 times the pixels' 800 MB for 16 bit RGBA (the
stream 1x, the two rows up to 2x, pix 1x): 3.2 GB measured for
100000000x1; an interlaced image's passes are at most half its width,
so its rows add at most 1x beside the stream, the pass and the full
image (not measured at its worst shape). A fix for the row term:
refuse or stream rows wider than a bound, or unfilter in place in the
inflated stream instead of a copy. A program taking
images from peers should pass a max_pixels for its use or check
decode_config first, as Go's documentation advises (Go has no limit).

### G2 (differences from Go, not faults)

- A stream that inflates past the image's size is refused with
  `zlib: output exceeds max_size (N bytes)` (a PngErr.Flate), where Go
  says `png: invalid format: too much pixel data`: when the first
  piece partial_inflate writes already passes the limit, it keeps no
  rows, so raw is shorter than the image and decode_image (1086) does
  not reach its `too much pixel data` branch. Same refusal, same cost;
  the fuzz harness and the stress program count stream errors as one
  class (pf_stream), so they do not see it. Fix: answer `too much pixel
  data` for a max_size error whatever partial_inflate kept, after the
  rows it did keep decode without error.
- decode refuses a header whose rows sum past 2^62 bytes as
  `dimension overflow` where Go reads on (F1's row bound): Go then
  answers `unexpected EOF` for a header only file and panics
  (`makeslice: len out of range`) when an IDAT follows. The row bound
  runs before the IHDR's CRC check, so such a header with a bad CRC is
  `dimension overflow` where Go says `invalid checksum`. decode_config answers Go's on every
  header since F1's fix (it answered a size for the pixel counts Go
  refuses before).

### Fault sites

- decode (175): `img ?? fault("png: no image")`. Not reachable: walk
  leaves its loop only at IEND, which needs stage DS_IDAT (1290), and
  the IDAT branch that sets DS_IDAT outside decode_config either sets
  img or returns an error (1264-1278).
- expected_size (710, 716), read_pass (749), decode_rows (1117,
  1119): not reachable since F1's fix: expected_size by walk's row
  bound (1186-1189, rows summing past 2^62 refused), read_pass's and
  decode_rows' `w * h * mbytes` by parse_ihdr's check (522-524: w * h
  at most 2^60 - 1, times model_bytes at most 8, at most 2^63 - 8; no
  headroom, so a model wider than 8 bytes needs a new bound).
- walk: `parts[0]` (1256) after the IDAT loop's first pass, which
  always pushes a part (the chunk at pos is an IDAT with 8 bytes
  there); every be32 follows a have() check (1169, 1231-1247), and
  `data[end + 8..data.len()]` follows have(data, end, 8).
- parse_ihdr, parse_plte, parse_trns: every byte read follows a have()
  check; parse_trns writes `d.palette[i]` for i below a length of at
  most 256 into a 256 entry palette; `transparent` is 6 entries, indexed
  0 to 5.
- read_pass and convert_row: raw reads follow the row's have() check;
  the row buffer is 2n bytes and each color type reads at most n bytes
  of its row (n is the packed row size for width); `i - bpp` only for
  i >= bpp.
- merge_pass: writes stay inside w * h * mbytes (a pass pixel (x, y)
  maps to (x * xf + xo, y * yf + yo), inside the image by pass_size).
- palette_of: at most 256 (npal and max_index are at most 256).
- partial_inflate: zdata[0] and zdata[1] after the length check.
- encode (200) faults when pix is not width * height * bytes, and
  `w * h * mb` itself can overflow for a caller's width and height near
  2^32; image (89) faults for a size under 1 and at (103) for a point
  outside the image. These are the program's own values: an Image from
  decode always agrees with its sizes (pix comes from read_pass and
  decode_rows at exactly w * h * mbytes). A program that builds an
  Image from a peer's dimensions and bytes checks them first. Caller
  contracts, documented on encode, image and at.
- No `!` unwrap or `or_fault`.

## 5. Defaults

- decode's max_pixels is 100,000,000 by default; a decode at that size
  can hold up to about 3.2 GB (G1). Go has no limit. A caller can lower
  it, or call decode_config first. A caller raising it to Int max (no
  limit, Go's behaviour) gets Go's `dimension overflow` for the headers
  an Int cannot hold (F1, fixed).
- decode_config reads the header only and allocates nothing from it.
- Every chunk's CRC is checked, ancillary ones included, as Go; no
  option skips it.
- No option weakens a check.

## Sign-off

Second reader: a subagent of Xo Lang 2 (2026-11-14), reading the code
at 97fc633a8 separately (its line numbers below moved to 002149649's): needs changes (two: G1's ceiling left out
read_pass's two row buffer, 3.2 GB measured for a 100000000x1 16 bit
RGBA file at the default limit; F1's proposed bound `(2^63 - 1) / 8`
still lets expected_size overflow for (2^30 + 1) x (2^30 - 1)), both
made above. It confirmed F1's overflow site and message (710,
2147483647 * 17179869177), that every other fault site is classed
correctly (walk's parts[0] at 1256, the have() guards, the 256 entry
palette, merge_pass's bounds), the line references (a few range ends
off by one or two: parse_trns ends at 652, read_pass at 825,
decode_rows at 1144), and G2's 2^62 config bound and max_size branch
(1085-1087). F1 was open at 97fc633a8; the review is redone at
002149649 below.

Third reader: a second subagent of Xo Lang 2 (2026-11-14), reading the
code at 002149649 separately: pass after five text fixes, all made
above (F1's tense at 97fc633a8; `w * h * mbytes` held by Go's check at
522, not by the row sum; Go's makeslice panic when an IDAT follows;
encode 192-321 and decode_image 1063-1097; the measure line's G2). It
confirmed that F1 is fixed: no product in expected_size, read_pass,
decode_rows, merge_pass or the interlaced sum overflows Int for any
header that passes parse_ihdr's check (522-524) and walk's row bound
(1186-1189), on 180,720 headers at both bounds and with a probe at
max_pixels Int max with a real IDAT, interlaced and not; the largest
product is decode_rows' `w * h * mbytes`, 2^63 - 8, for 16 bit gray or
truecolor with tRNS. decode_config refuses the headers Go's
DecodeConfig refuses, in Go's order of checks, an invalid depth with a
huge size included. About 45 line references checked at 002149649.
