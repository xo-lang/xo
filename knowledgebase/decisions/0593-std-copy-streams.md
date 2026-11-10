# 0593: Copying between streams and files (Go's io.Copy)

Status: Accepted (lead, 2026-11-10, on Lang 3's recommendation; the owner
may override at review). Stage 1 shipped; stage 2 (sendfile) open.

## Context

The owner approved filling the std audit's gaps (lead, 2026-11-10);
bench/results/std-audit-2026-11-10b.md ranks io.Copy fourth by use. Go
programs use it to move bytes between two connections (proxies), a file
and a connection (serving or saving a body), and two files. Xo had the
pieces but no copy: `Stream` (decision 0481) reads and writes with
`NetErr uses net`, `File` with `FsErr uses fs.read`/`fs.write`, and
`Fs.copy` copies a path to a path.

The two kinds differ in error type and effect, Xo has no method values
(`src.read` is not a function value) and no union of two error types, so
one generic `copy(dst, src)` over anything with read and write is not
expressible without a language change.

## Decision

```
enum CopyErr[R, W] {
  Read(err: R)
  Write(err: W)
} derive Display

fn copy_stream(dst: Stream, src: Stream) -> Int ! CopyErr[NetErr, NetErr] uses net
fn (f: File) copy_to(dst: Stream) -> Int ! CopyErr[FsErr, NetErr] uses (fs.read, net)
fn (f: File) copy_from(src: Stream) -> Int ! CopyErr[NetErr, FsErr] uses (fs.write, net)
```

Each reads 32 KiB at a time (Go's io.Copy buffer), writes each piece in
full, and returns the bytes copied. CopyErr names the side that failed and
carries that side's own error: Go's io.Copy returns one error and the
caller cannot tell, which matters to a proxy.

The Stream to Stream copy is a function, not the `Stream.copy_to` method
first proposed: an interface takes no methods declared outside its block
(XO0601), and a method inside it would be one every user implementation
of Stream must write. Its arguments are in io.Copy's order (dst, src).
File to File stays `Fs.copy` (by path).

Stage 2 ports Go's fast path: Go's TCPConn.ReadFrom uses sendfile(2) for
a file source (darwin and Linux) and splice(2) between sockets on Linux.
`File.copy_to` of a real file into a native connection uses sendfile,
measured with `xo bench --ab` on the linux/arm64 host.

## Consequences

- Prelude: CopyErr, copy_stream, File.copy_to, File.copy_from; native
  bodies in std/native/prelude.xo, the Go backend's in std/rt/copy.go.
- Golden copy_stream ports io_test.go's TestCopy (here three 32 KiB
  reads over loopback), an empty source, a read error (Read with the
  connection's NetErr) and a write error (Write: a file open for reading).
- Timing (Mac, 64 MiB over loopback, median of 5): conn to conn 52 ms
  against io.Copy's 40 ms; file to conn 42 ms against 14 ms, Go's
  sendfile path, which stage 2 ports.

## Alternatives

- **A generic copy over two closures** (`copy(fn(n) { src.read(n) },
  fn(b) { dst.write(b) })`): one function, but the call reads worse than
  Go's, and both closures are written at every call.
- **Reader and Writer interfaces** with error type parameters: the
  general answer, a language change larger than this gap.
- **Only Stream to Stream**: covers proxies, not the file cases.
