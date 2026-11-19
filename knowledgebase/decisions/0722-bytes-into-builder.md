# 0722: Bytes.into_builder takes over a uniquely owned Bytes without a copy

Status: Proposed (xo-lang-2, 2026-11-19, under task #271; the lead puts it
to the owner through xw decide)

## Context

std/image's draw of an RGBA source onto an RGBA destination (draw_rgba_rows)
builds a new BytesBuilder of dst.pix.len() on every call, copies the old
pixels in, writes the drawn rows and freezes it back into dst.pix. Bytes is
immutable (spec/core.md 3.6, XO0502) and no prelude method turns a Bytes
into a BytesBuilder, so this copy is the only way to change a pixel.

Measured on the Mac (60 draws of `over`, 1024x768 RGBA, 2026-11-19): Xo
0.16 s, Go's draw.Draw in place 0.12 s, and Go with a new image plus a copy
on each pass (what Xo's draw does) 0.155 s. The pixel loop is at parity;
the gap is allocating and faulting in a new 3 MB buffer per draw. The
committed amd64 row is 1.064 of Go (435a260a9), the last image row above
#271's 1.05 bar.

Decision 0508 showed a List builder can reuse a dead receiver with no
compiler change (`var a = xs; a.sort_in_place()`): copy-on-write copies only
while a second binding holds it. That route does not reach Bytes: there is
no in-place writer on a Bytes, and std/image cannot use std/native/llrt/raw
(native only). The runtime already writes in place into a string object
whose count is 1 and whose capacity covers the need (decision 0402), and
BytesBuilder.put_be and put_le (decision 0399) store over existing bytes in
place. What is missing is the handover from Bytes to BytesBuilder, and a way
to read the builder's bytes while writing them (Over reads each destination
pixel before it stores the result; reading through a second `let dp =
dst.pix` binding would make the count 2 and force the copy again).

## Decision

Two prelude additions, no language or checker change:

1. `fn (b: Bytes) into_builder() -> BytesBuilder`: a builder holding b's
   bytes, which moves b. It takes b's object without a copy when the object
   has count 1, is not a view (decision 0330 offset 0, length the whole
   object), and is not a read-only mapping (decision 0402); otherwise it
   copies, as `BytesBuilder.with_capacity(b.len())` plus `write(b)` does
   today. The result is the same either way; only the copy differs.
2. `fn (b: BytesBuilder) get_be(at: Int, n: Int) -> Int` and `get_le`, the
   readers matching put_be and put_le (n is 1 to 8, at + n within len(),
   else a panic as put's checks), plus `fn (b: BytesBuilder) at(i: Int) ->
   UInt8`.

spec/std.md text (BytesBuilder section):

    fn (b: Bytes) into_builder() -> BytesBuilder
      // a builder that starts with b's bytes; moves b. No copy when b
      // has no other owner, is not a slice of a larger Bytes and is not
      // mapped from a file (fs.read_mapped); else b is copied
    fn (b: BytesBuilder) at(i: Int) -> UInt8
    fn (b: BytesBuilder) get_le(at: Int, n: Int) -> Int
    fn (b: BytesBuilder) get_be(at: Int, n: Int) -> Int
      // the n bytes (1 to 8) at at, as put_le and put_be store them

std/image then draws with `var b = dst.pix.into_builder()`, reads and
writes through b, and sets `dst.pix = b.freeze()`. A dst whose pix is
shared (another Image value still holds it) copies once, as today.

## Consequences

- image draw of RGBA onto RGBA stops allocating per call when dst is the
  only owner of its pixels; expected near Go's in-place 0.12 s (to be shown
  with `xo bench --ab` on host 1 before the std change claims it).
- Other std users of the copy-then-freeze shape (png and jpeg writers
  filling a buffer, binary's put helpers) can adopt it; each is its own
  change with its own measurement.
- Mapped views stay read-only: into_builder of a read_mapped Bytes copies.
  Open item for Lang 1: how into_builder tells a mapped object from a
  unique heap object whose capacity equals its length (0402 marks mapped
  objects only by capacity == length, which a frozen with_capacity(n)
  builder also has). Options: a flag bit in the header, or the allocator's
  8 byte prefix. This is the one compiler/runtime piece and goes to Lang 1
  as a small task.
- wasm and the Go backend implement into_builder as the copy (always
  correct); the in-place path is native only at first.

## Alternatives

- `Image.draw` writing through raw (std/native/llrt): image is portable std
  and cannot reach raw; it would also put an unchecked writer in a module
  that has none.
- Keeping pix as List[UInt8]: a List already reuses a unique receiver
  (0508), but Image.pix is Bytes in the public API (Go's []byte), and every
  encoder and decoder reads it as Bytes; changing it is a breaking change
  for every image user.
- Leave the 1.064: #271's bar is 1.05 for cases above 5 ms, and the cause is
  known and small to fix.
