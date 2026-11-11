# 0600: File.seek

Status: Proposed (Xo Lang 3, 2026-11-11), for the owner. Built on branch
lang3-seek while it waits.

## Context

The std audit of 2026-11-11 (bench/results/std-audit-2026-11-11.md) ranks
"no File.seek" fourth (score 38: `io.SeekStart` in 24 stdlib files,
`os.File.Seek` in 14). A File reads and writes sequentially, or at an
offset with `read_at` and `write_at` (decision 0479), which leave the
position alone. Code ported from Go that rewinds a file (read a header,
seek back, hand the file to a decoder), skips a region, appends after
reading the size, or finds the size with `Seek(0, io.SeekEnd)` has no
direct form: it reopens the file or switches every read to `read_at`
with its own offset.

A File's sequential reads are buffered (64 KiB, as Go's bufio.Reader in
std/rt fsfile.go; natively std/native/llrt/fsops.xo's F_BUF, F_R, F_W),
so the position the OS holds is ahead of what the program has read.

## Decision

```
enum Whence { Start, Current, End }          // io.SeekStart, SeekCurrent, SeekEnd
fn (f: File) seek(offset: Int, whence: Whence = Whence.Start) -> Int ! FsErr uses fs.read
```

- Returns the new offset from the start of the file, as Go's Seek.
- `Current` counts from what the program has read, not from the OS
  position: the unread bytes of the read buffer are subtracted, and the
  buffer is discarded (Go's own os.File has no buffer; a program that
  wraps one in bufio.Reader calls Reset after a Seek, which this does
  itself).
- A resulting offset below 0 is an error with Go's text (`seek PATH:
  invalid argument`, EINVAL); an offset past the end is allowed (a later
  write leaves a gap that reads as zeros, as Go).
- Works on every open mode: reads, writes, open_rw, and open_append
  (where, as in Go and O_APPEND, a write still goes to the end).
- A closed file: Go's `file already closed` text, as the other methods.
- MemFs (testing) files keep a position and seek the same way.
- Layers: lseek on Linux (62 arm64, 8 amd64) and FreeBSD (478), libSystem
  lseek on macOS, SetFilePointerEx on Windows (already bound), the wasm
  runtime's fs_file_seek, Go's os.File.Seek on the Go backend.

`seek(0, Whence.End)` gives the size from the handle; `stat().size`
remains the plain way.

## Consequences

- Rewind, skip and size-from-handle port directly from Go.
- A new prelude enum (`Whence`) and method; spec/std.md 3.1 and the
  prelude listing gain them. A new rt intrinsic for wasm (rt.fs_file_seek)
  is lowered in wasm's rt2.xo too.
- The read buffer's discard costs a refill after each seek; a program
  that seeks often between small reads should use `read_at`.

## Alternatives

- No seek, document `read_at`/`write_at` (status quo): every port that
  rewinds rewrites its reads around offsets; the audit counts 38 files.
- `seek(pos: Int)` only (absolute): covers rewind but not size from the
  handle or relative skips; Go's three forms cost one enum.
- An integer whence (Go's 0, 1, 2): an enum makes a wrong value
  unrepresentable.
- `io.SeekStart` constants in a separate module: Xo has no io module;
  File's own enum keeps it beside the method.
