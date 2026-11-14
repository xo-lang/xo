# 0635: a Decompressor write bounded by the caller (Go's Read)

Status: Proposed (Xo Lang 2, 2026-11-14; target 0.52)

## Context

std/compress's `Decompressor.write(data)` returns everything its input
completes, so one write's output is bounded only by `max_size` (default
1 GiB), never by the size of the write: DEFLATE gives up to about 1032
bytes a byte. Go's readers are pull style, and the caller's buffer bounds
what one `Read` decodes. The std/compress security review (decision 0616,
std/evidence/compress/review.md, Lang 3's G2) measured a 260 KB gzip of
256 MB of zeros fed in 32 KB writes returning up to 35.9 MB from one
write. 0.51.1 bounds std's own streaming caller (std/http's client feeds
4 KiB at a time, GZ_SLICE) and documents the bound in spec/std.md; this
decision is the API that lets every caller bound a step.

## Decision

To be written: a write that returns at most a given number of bytes and
keeps the rest decoded lazily (InStream already resumes inside a block
since the F1 fix), its name and default, how `finish` drains what is
held, and whether std/http's client and std/png move to it.

## Consequences

To be written.

## Alternatives

- Keep feeding small writes in each caller (0.51.1): works, but every
  caller must know DEFLATE's ratio.
- A per-write output cap in the Decompressor's constructor.

With this change: update the section comment above InStream in std/native/llrt/flate.xo (about line 2622), which still describes the header reread of before F3 ("only once the input waiting has doubled"; gzip_need replaced it, and a dynamic header is read again on every push). Found by aws-43, std/evidence/compress/review.md; held back so a comment alone does not change the source hash the compress evidence names.
