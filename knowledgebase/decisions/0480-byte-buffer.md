# 0480: A byte buffer to consume from the front (Go's bytes.Buffer)

Status: Proposed (Xo Lang 2, 2026-11-03; for the owner; no implementation
before a choice)

## Context

The backend survey of 2026-10-30 (std-coverage.md) listed std level
obstacles beside its ranked gaps. Checked against main on 2026-11-03,
four of the five are already there:

- `List.filter_map` (std/prelude.xo, 5f339ffc8, 2026-10-30).
- A deque: std/deque (e71fc47f2, 2026-10-30).
- `backoff.retry`'s notify hook: `backoff.retry_notify(b, clock, rand,
  op, retryable, notify)`, whose `retryable` and `notify` may have effects
  of their own (e44acf4a8, 2026-10-30), Go's RetryNotify.
- A bit preserving Int to UInt64: `UInt64.from_bits(b: Int)` (b77c120c5,
  2026-10-30), Go's `uint64(x)`; `UInt64.try(x)` is the checked form.

The fifth is "no cursor to consume a `Bytes` from the front (each fill
copies the unread rest)". Part of it is gone too:

- A slice of Bytes is a view since decision 0330, so `rest = b[n..]`
  copies nothing.
- std/bufio (decision 0471) reads records from any `Stream` with the
  record a view of its chunk.

What is left is a source that is not a Stream, or bytes pushed rather
than pulled: a `Decompressor.write` result, a `WsConn` message stream, a
`BodyReader`, a `File`, a protocol framer fed by a caller. Then the
program keeps the unread part and appends the next piece, `rest + more`.
That copies the unread rest on every fill. A record larger than one piece
(a 1 MiB RESP bulk string in 4 KiB reads) costs time quadratic in its
size. Go's answer is bytes.Buffer: Write appends, Next(n) and ReadBytes
consume, and the unread part slides to the front only when the buffer
must grow (amortized constant work per byte).

## Decision (proposed; options below)

Option B1, a prelude `ByteBuffer`, Go's bytes.Buffer cut to what a parser
needs:

```
/// Bytes appended at the back and consumed from the front; move only.
struct ByteBuffer {}
fn (static: ByteBuffer) new() -> ByteBuffer
fn (static: ByteBuffer) with_capacity(n: Int) -> ByteBuffer
fn (var b: ByteBuffer) write(data: Bytes)
fn (b: ByteBuffer) len() -> Int                         // unread bytes
fn (b: ByteBuffer) peek(n: Int) -> Bytes                // up to n unread bytes, not consumed (a view)
fn (var b: ByteBuffer) next(n: Int) -> Bytes            // up to n bytes, consumed (Go's Next)
fn (var b: ByteBuffer) read_until(delim: UInt8) -> Bytes?   // through delim, consumed; None if absent (nothing consumed)
fn (var b: ByteBuffer) discard(n: Int)
fn (b: ByteBuffer) find(sub: Bytes) -> Int?             // offset in the unread bytes
```

- `peek`, `next` and `read_until` return views of the buffer's storage.
  The storage is never written under a view: a `write` that needs room
  either appends past the end or moves to new storage (the old storage
  stays alive while a view holds it, by reference counting). So a view
  stays valid and no copy is made at read time. Go's Next documents the
  opposite (valid only until the next write); Xo's views cannot dangle,
  so it does not need the rule.
- Growth: when the unread part is at most half the storage, `write`
  moves it to the front of storage of its own only if no view holds the
  old one (the count says so). Otherwise it allocates the larger of
  twice the unread part plus the new data and copies the unread part
  once. Each byte is copied O(1) times amortized, as Go's grow.
- On the Go backend it wraps bytes.Buffer, with the copy a view needs
  (Go's Next result is only valid until the next write).
- `bufio.Reader` stays the form for a `Stream`. A `bufio.reader_from(src:
  fn(Int) -> Bytes? ! E)` over any pull source is B2 below.

## Options

- **B1, `ByteBuffer` in the prelude (proposed).** It covers push sources
  (a Decompressor, a caller's framer) and pull sources (a loop that
  writes what it reads). Go's type and names.
- **B2, a pull source for bufio** (`bufio.reader_from(fn(max) -> Bytes?
  ! E)`). It reuses std/bufio's tested record reading for File,
  BodyReader and WsConn payloads, but not for a push source: a
  Decompressor's output cannot be pulled without a buffer between them.
  This is worth doing as well, on top of B1 or alone.
- **B3, nothing new: views and an offset.** Today's Bytes views let a
  program keep `(data, off)` and concatenate only when a record spans
  pieces. That is correct and cheap if written carefully. The survey's
  service did not write it so, and every parser repeats the bookkeeping.
- **B4, a `Bytes` cursor type** (Go's bytes.Reader: read, seek, unread
  over fixed bytes). Views already give that for fixed bytes; it does
  not help appending.

## Measurement plan

Peak memory and wall time; linux/arm64 host 1, `xo bench --ab` with
a Go twin (bytes.Buffer with ReadBytes and Next), 15 rounds,
bench/perf/bytebuf with a twin.txt:

1. **Small records:** 1,000,000 RESP-like lines of 20 to 80 bytes,
   delivered in 4 KiB pieces. Compare `rest + more` and `read_until`
   (today), views with an offset (B3), `ByteBuffer.read_until` (B1) and
   Go. Expect B1 within 1.2x of Go and of B3, and below `rest + more`.
2. **One large record:** a 1 MiB bulk string in 4 KiB pieces, then 64
   MiB. Expect `rest + more` quadratic (its wall grows 4096x for 64x the
   size) and B1 linear, near Go.
3. **Held views:** keep every 100th record (a view) while parsing 1,000,000.
   Peak RSS against Go, which copies kept records. This checks that
   moving storage while views hold it does not retain more than about 2x
   the kept bytes.
4. **Correctness:** Go's bytes/buffer_test.go cases for Write, Next,
   ReadBytes and Len ported into a corpus (scripts/stdcorpus), and a
   seeded random sequence of writes and reads against bytes.Buffer, on
   every backend.

## Consequences

- New prelude type `ByteBuffer` (spec/std.md 2, std/changes/prelude.md),
  on the native backends (std/native/prelude.xo) and the Go backend.
- The survey's std level obstacles are all closed once this lands. Its
  language level ones go to the language owner (std-coverage.md).

## Alternatives

The options above; the proposal is B1, with B2 as a follow-up.
