# 0471: std/bufio, a buffered reader over a Stream (proposal)

Status: Proposed

## Context

The 2026-10-30 backend survey (std-coverage.md, rank 8) found that every line protocol (the Redis wire
protocol queue worker was the case) hand-writes a buffer over `Conn.read(max)`: it must look for `\r\n`,
keep the bytes after it for the next call, and read exactly N bytes for a bulk string. Go's answer is
`bufio.Reader` (ReadString, ReadSlice, Peek, io.ReadFull). Xo has `File.read_line` (prelude) but nothing
for a `Conn`, a `ServerConn` or any `Stream`, and `Bytes` has no cursor to consume from the front (each
refill copies the unread rest), so the loop each service writes is quadratic on long lines.

## Options

1. `std/bufio`, one type over the `Stream` interface (so a `Conn`, a `ServerConn` and a TLS connection all
   work):

   ```xo
   enum BufErr { Net(err: NetErr), TooLong, UnexpectedEof }       // display() as the others
   struct Reader {}
   pub fn reader(s: Stream, size: Int = 4096) -> Reader
   fn (var r: Reader) read(max: Int) -> Bytes? ! NetErr uses net         // buffered bytes first, else one read
   fn (var r: Reader) read_until(delim: UInt8, limit: Int = 1048576) -> Bytes? ! BufErr uses net  // delimiter kept; None at a clean end
   fn (var r: Reader) read_line(limit: Int = 1048576) -> Str? ! BufErr uses net   // "\n" or "\r\n" removed
   fn (var r: Reader) read_exact(n: Int) -> Bytes ! BufErr uses net     // io.ReadFull: UnexpectedEof when short
   fn (var r: Reader) peek(n: Int) -> Bytes ! BufErr uses net           // does not consume; fewer than n only at the end
   fn (r: Reader) buffered() -> Int
   ```

   `read_until` and `read_line` fail with `TooLong` past `limit` bytes without a delimiter (Go's
   ErrBufferFull is its size; a limit that is not the buffer size is what servers want, and it bounds
   a hostile peer's memory). `read_line` of invalid UTF-8 is lossy (U+FFFD), as `File.read_line`.
   The reader keeps one `Bytes` and an offset, refilling by reading at least what is missing, so a long
   line costs one copy of itself, not one per read. A `Reader` is a move-only handle (a `var`
   receiver, as `xml.Reader`).
   Cost: std/bufio.xo declarations, a native implementation over the `Stream` interface (about 120
   lines), the Go backend's (an `interface` method call per refill), spec/std.md section, std/changes,
   corpus (a fake Stream that returns data in prescribed chunk sizes: 1 byte at a time, chunks across
   delimiters, a `\r` at a chunk end; compared against Go's bufio.Reader on the same chunking), a timing
   row against Go's bufio.Reader over a net.Pipe.
2. Add the same methods straight to `Conn`, `ServerConn` and `Stream`: no second type, but each handle
   then carries a buffer, and an interface cannot hold state; rejected.
3. A buffered reader in each service, as today: works, quadratic when written naively, and 3 of the 5
   surveyed services had to.
4. Wait for a language level cursor over `Bytes` (survey "language obstacles"): fixes the copy but not
   the line and exact-read protocol logic every service rewrites.

## Recommendation

Option 1. The method names are Go's bufio.Reader's (ReadBytes/ReadString -> `read_until`/`read_line`,
Peek, io.ReadFull) with Xo's error type, so a port reads the same.

## Consequences

A new public module (hence this ADR). `BufErr` wraps `NetErr` so callers still match transport failures
(Timeout, Canceled) the way they do on `Conn.read`; the deadline of the underlying connection applies to
each refill. A bufio.Writer is not part of this: `Conn.write` writes everything, and batching is the
caller's `BytesBuilder`.

## Alternatives

Listed above (2, 3, 4). Also rejected: a `Scanner` with split functions (Go's bufio.Scanner), a larger
API for the same need; it can be added over the reader later.
