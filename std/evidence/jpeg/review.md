kind: review
module: jpeg
result: pass
commit: 58b0c68a4
source: 019450388a9c7ef8436c7951c465cf4117b5e1755aad4b2ecc608bb05d56b1d4
date: 2026-11-14
host: darwin/arm64 (probes and tests), linux/arm64 host 2 (fuzz, stress, race), host 1 (perf)
command: none (section 3 does not apply: no secret); read against knowledgebase/infrastructure/std-security-checklist.md
measure: 0 Go vulnerability entries for image/jpeg (182 stdlib entries read, 2026-11-14; the 13 golang.org/x/image entries are tiff, webp, vp8l, bmp and sfnt, read as siblings); 2 inputs a peer controls (a file to decode or decode_config, an image to encode when a program encodes a peer's pixels), each bounded: max_pixels (100,000,000 by default; Go has none) before any allocation, segment lengths checked before every read; 1 defect found and fixed (F1: a frame header asking for a large image cost the whole image before its scans were read, 1.1 s and 1.5 GB for a 20 KB file where Go takes 3 ms, 35a44e04f) and 1 bound documented (G1: a valid image costs its pixels, as in Go, up to max_pixels); 4 fault sites, all in encode on an Image whose sizes disagree, a caller contract; 0 insecure defaults

Security review of std/jpeg (std/jpeg.xo, which the Go backend also
compiles, decision 0312) against
knowledgebase/infrastructure/std-security-checklist.md (decision 0616),
by Xo Lang 3. std/jpeg is a port of Go's image/jpeg decoder (reader.go,
huffman.go, scan.go, idct.go of go1.26) over whole buffers, with Go's
encoder (decision 0510): decode, decode_config and encode (spec/std.md
49). It reads files a peer sends: uploads, avatars, images fetched from
a URL.

## 1. Go's CVE history

The Go vulnerability database (vuln.go.dev, the 182 stdlib entries
fetched on 2026-11-14) has no entry for image/jpeg or any image/*
package of the standard library.

Sibling classes, from the golang.org/x/image entries (read 2026-11-14):

- Allocation from a declared size before the data is read (GO-2026-4815
  tiff IFD offset, GO-2026-5062 tiff tile sizes, GO-2026-6222 vp8l,
  GO-2026-4962 sfnt): F1 is this class, in a form Go's lazy zeroed pages
  hide. decode refuses a header over max_pixels before allocating
  (jpeg.xo process_sos, `image too large`), which Go does not.
- CPU on degenerate sizes (GO-2023-1990 tiff 0-height images,
  GO-2023-1989): a zero width or height is not refused (neither is it in
  Go's reader.go); it gives no MCUs (mxx or myy 0) and an empty image at
  no cost; a scan's work is bounded by the blocks the frame declares
  (at most max_pixels / 64 per component, with sampling factors summing
  to at most 10, jpeg.xo:1476 `total sampling factors too large`).
- Panics on out of range indexes (GO-2024-2937 tiff palette, GO-2026-5031
  bmp palette, GO-2026-5061 webp alpha): section 4, no reachable fault.

## 2. Limits on hostile input

| Input a peer controls | Limit | Where | Test at and past it |
|---|---|---|---|
| Pixels a frame header declares | max_pixels, 100,000,000 by default, checked before any plane or coefficient is allocated; after F1 the memory and time follow the scans actually decoded | jpeg.xo decode (144), the max_pixels check (1502-1504, `>`: a frame of exactly max_pixels passes), make_img (1068), ensure_plane (1117), coef_set and ensure_coef (1152, 1161) | std/jpeg_test.xo: 10000x10000 headers with little data at the limit (Go's errors; under 1 ms and 4 MB on a Mac) and one pixel past it (max_pixels 99,999,999: `image too large`); corpus jpeg |
| Segment lengths (DHT, DQT, SOF, SOS, DRI, APPn, COM) | each read checks the bytes are there first (read_full, Go's readFull), and each segment's own length rules as Go (`DHT has wrong length`, `DQT has wrong length`, Huffman `excessive length`) | jpeg.xo segments (1861-1954), process_dht (800-883), process_dqt (885-932) | fuzz TestJpegFuzz (Go's test images and encodes with bytes changed, cut and repeated, 1000 per seed against Go's errors); corpus jpegfuzz |
| Table and component indexes (Tc, Th, Tq, Td, Ta, component count) | refused past Go's bounds (`bad Tq value`, `bad Td value`, `bad Ta value`) | process_dht (810-815), process_dqt (895), process_sof (980-1060), process_sos (the Td and Ta checks before the max_pixels check) | fuzz above |
| Huffman codes, EOB runs, successive approximation | Go's checks: `bad Huffman code`, `too many coefficients`, `excessive DC component`, `bad spectral selection bounds`, `bad successive approximation values`; values wrap to 32 bits where Go's int32 does | decode_huffman, refine, process_sos | fuzz above; stress std/evidence/jpeg/stress (64 files, half changed as the fuzz changes them) |
| An image a program encodes from a peer's pixels | width and height below 1 << 16 (`image is too large to encode`, Go's) | encode (179-234) | fuzz TestJpegEncFuzz; corpus jpegenc |

## 3. Constant time

Not applicable: std/jpeg handles no secret.

## 4. Fault behaviour

### F1 (fixed in 35a44e04f): a large frame header cost the whole image before its scans

decode allocated the planes and, for a progressive image, every
coefficient of a component when its first scan began, sized by the frame
header: Bytes.zeroed writes every page and the builder copied it, and
the coefficients were pushed one Int32 at a time. Go's make returns
pages the system zeroes on first touch, so in Go such a file costs only
what its scans decode. A 20 KB progressive file (Go's
video-001.progressive.jpeg with its header changed to 10000x10000) took
1.08 s and 1.5 GB resident before failing with Go's error (Go: 3 ms,
8.6 MB); a 496 byte baseline file 27 ms and 204 MB (Go: under 1 ms,
5.9 MB). Mac, native. Bounded by max_pixels, so a cost per decode of
about 75,000 times the input, not unbounded. 0.38.0 to 0.51.1, every
backend.

Fix: the planes are reserved and written with zeros only up to the
block being stored (ensure_plane, 1117, doubling) and padded to their
size in finish (1779); a progressive image's coefficients are stored in
MCU order (coef_at, 1137), so each scan writes in MCU row order, and the
store grows by doubling only when a scan writes past its end (coef_set
and ensure_coef, 1152 and 1161), so an end-of-band run, which skips
blocks without a write, touches nothing; a coefficient past the end
reads as zero (coef_get, 1145; load_block, 1415). Both files now fail in under 1 ms with 4 MB resident.
Tests: std/jpeg_test.xo; TestStdCorpora jpeg, jpegenc and jpegfuzz on
go, arm64 and llvm; TestJpegFuzz and TestJpegEncFuzz; std/changes/jpeg.md
and the CHANGELOG security line.

### G1 (bound documented): a valid image costs its pixels, up to max_pixels

After F1, what a decode costs follows what its scans write, but a valid
file still costs its whole image: finish pads every plane to its size,
and reconstruct_progressive stores every block (a block no scan wrote
as the inverse DCT of zero, computed once, not per block; a baseline
block whose scan wrote no AC term is filled from its DC term without the
inverse DCT, 58b0c68a4). A file can be small for a large image (Go's
encoder writes a flat 10000x10000 gray image in 1.17 MB): decoding it
takes 184 ms on the Mac after 58b0c68a4 (393 ms and 108 MB after de2feac05, 460 ms
before), Go 223 ms and 105 MB, so memory as Go's and time below Go's on
this image. Go has no
limit; max_pixels bounds it here. The largest a decode can hold at the
default 100,000,000 pixels: the planes (3 bytes a pixel at 4:4:4, 4 for
CMYK) plus, for a progressive image, the Int32 coefficients (12 bytes a
pixel at 4:4:4, 16 for CMYK), and while the store doubles, List.concat
holds the old store beside the new one: about 2.7 GB for a progressive
4:4:4 image of the full size, where Go's int32 blocks take the same 12
bytes a pixel. A program taking images from peers should pass a
max_pixels for its use or check decode_config first, as Go's
documentation advises. The 4 MB measured for a failing file also relies
on with_capacity reserving the planes without touching them (Xo's
allocator maps large blocks lazily; Bytes.zeroed writes them, which was
F1's cost).

### Fault sites

- encode (187, 190, 193, 233) faults when an Image's pixel or plane
  sizes disagree with its width and height, or a size is negative. An
  Image from decode always agrees; a program that builds one from a
  peer's dimensions and bytes must check them, as Go's encoder reads
  out of range on such an image. A caller contract, documented on
  Image (36-50).
- Decode has no fault call: every byte_at (798) of a segment follows
  read_full's check, and read_byte_stuffed checks two bytes ahead before
  it reads; table indexes are checked as listed in section 2;
  `UNZIG[zig]` is read only for zig in 0..63 after the spectral bounds
  check.

## 5. Defaults

- decode's max_pixels is 100,000,000 by default; a decode at that size
  can hold up to about 2.7 GB (G1). Go has no limit. A caller can lower it, or call decode_config first,
  as Go's documentation advises.
- No option weakens a check.

## Sign-off

Second reader: a subagent of Xo Lang 3 (2026-11-14), reading the code
separately: needs changes (the cost left after F1, the peak at the
default limit, the zero size claim, a test past the limit, stale lines),
all made above; the end-of-band and flat block steps were added to F1's
fix after its read; it confirmed every byte_at follows read_full, UNZIG
stays in 0..63, and the lazy store's addressing (pb + 64 never past the
frame's total in either scan order, no written block read back as zero).
