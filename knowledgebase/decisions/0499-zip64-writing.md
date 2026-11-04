# 0499: std/archive writes zip64 as Go does

Status: Proposed (Xo Lang 2, 2026-11-04, at the lead's request). Reverses
the documented "No zip64" write limit of spec/std.md 18; held for the
owner before any code.

## Context

std/archive reads zip64 archives on every backend (decision 0172, since
2026-10-21) but writes none. spec/std.md 18 documents the limit: more
than 65,535 entries or 4 GB is an error. A differential run on
2026-11-04 (600 generated archives, 11,510 entries, read by Go's
archive/zip with 0 failures) found that the backends did not agree past
it:

- the native `zip` wrote the end record's 16-bit count modulo 65,536 (an
  archive of 65,537 entries said 1), with no error;
- the Go backend's `zip` and `zip_writer` let Go's writer switch to zip64
  and succeeded, against the spec.

The fix pushed for 0.39 (golden zip_entry_limit) makes every backend
refuse the 65,536th entry and an archive past 4 GB. That is consistent
but is a limit Go does not have: a program that archives a large tree
(a build cache, a log directory, a backup) fails where Go's code works.
The AWS SDK and the owner's evaluations port Go programs, so they meet
it as a difference.

## Decision (proposed)

Port the zip64 half of Go's archive/zip Writer to std/native/archive.xo
and remove the errors on every backend:

- `zip` and `zip_writer` write a zip64 end of central directory record
  and its locator when the entry count is at least 65,535 or the
  directory's size or offset is at least 0xffffffff, storing the maximum
  values in the classic end record (Go's Writer.Close).
- An entry whose compressed or uncompressed size is at least 0xffffffff,
  or whose header offset is, gets the zip64 extra field (ID 0x0001) in
  its directory record with the 32-bit fields at 0xffffffff, reader
  version 45, and a 24 byte data descriptor (Go's fileWriter.close and
  writeDataDescriptor).
- The output is byte identical with the Go backend's (which is Go's
  writer), checked by the differential harness and goldens at 65,535,
  65,536 and 65,537 entries, plus one run with an entry past 4 GB
  (deflated zeros, so the archive stays a few MB) on an offload host.
- spec/std.md 18: "No zip64" becomes "past 65,534 entries or 4 GB the
  archive is zip64, as Go writes it"; std/changes/archive.md and
  CHANGELOG get the entry.

## Consequences

- Programs that archive more than 65,535 files or 4 GB work as in Go.
- A zip64 archive needs a reader that knows zip64 (every current tool
  does: Go, Info-ZIP 6, Python, 7-Zip, macOS Archive Utility); an archive
  under the limits is unchanged byte for byte.
- About 60 lines in std/native/archive.xo (zip_central, zip_descriptor,
  the size checks); the Go backend loses its two refusals.

## Alternatives

- Keep the limit (today, after the 0.39 fix): consistent and documented,
  but a difference from Go with no benefit beyond not writing the code.
- zip64 only for the entry count, not sizes: the 4 GB limit is the one a
  streaming `zip_writer` (decision 0479) exists for, so it would keep the
  more likely failure.
