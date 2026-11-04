# 0480: A byte buffer to consume from the front (Go's bytes.Buffer)

Status: Accepted, B1 (owner, 2026-11-03, through the lead, after reading
the three loops side by side: B3 is fast but asks every parser for about
25 lines of bookkeeping, while B1 reads like the naive loop without its
quadratic trap). Gate: bench/perf/bytebuf's `_buf` no slower than B3's
`_views` (host 1 --ab), peak RSS at or under Go's bytes.Buffer on small,
large1 and large64, Go's buffer_test ported, every backend including the
Go wrapper with its view copy. B3's numbers below are the bar. An earlier
status, Accepted on a condition with B3 meeting it, sent the choice back
to the owner. Shipped (owner, 2026-11-03): judged against B3 doing the same
work (`_vrec`, each record made as a view), B1 is 1.049 on 50 byte
records and equal on large ones, with B3's peak RSS; against `_views`
(records only counted) it is 1.074. Both bars are in
bench/results/std-twin-bytebuf-2026-11-03.md.

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

## Decision

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

- **B1, `ByteBuffer` in the prelude (chosen).** It covers push sources
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

## Measurement (2026-11-03)

bench/results/std-twin-bytebuf-2026-11-03.md, linux/arm64, Go's
bytes.Buffer as the twin: option B3 (views and an offset, no new type)
cuts 1M small records at 0.916 of Go's wall and 0.182 of its peak RSS, a
1 MiB record at 0.171 of its peak (wall within noise), and a 64 MiB record
at 0.201 of its wall and 0.249 of its peak. Today's `rest + piece` is
0.919 on small records too, and quadratic on a large record only. B3
met the owner's first bar; the owner then chose B1 for the code a parser
writes, with these B3 numbers as the bar B1 must meet.

## Implementation (2026-11-03)

`read_until` keeps how far its last search for the same delimiter got, so
`while let Some(rec) = b.read_until(0x0A)` after each write is linear (Go's
ReadBytes rescans; the Go twin keeps that offset by hand). Natively a
write keeps its Bytes as a view, writes under 64 bytes are gathered in
one appended tail, and a record that spans pieces is joined once: B3's
work, behind B1's methods. A first form copied each piece into one
storage Str (moved to new storage once half was consumed, Go's grow) and
was 7% slower than B3 on small records (0.993 of Go's wall against
0.925, host 1). On the Go backend std/rt/bytebuf.go wraps bytes.Buffer
and copies each returned view.
