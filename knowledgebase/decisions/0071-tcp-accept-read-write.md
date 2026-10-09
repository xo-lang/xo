# 0071: TCP connections in the prelude: accept, read, write, close

Status: Accepted (2026-10-08). Declared in `std/prelude.xo`, implemented
in `std/rt/caps.go` (Go backend) and `std/llrt/xo_rt.c` with
`internal/ssa/build_io.go` (native, decision 0069). Golden:
`internal/llvm/testdata/golden/net_echo` (Go, LLVM, and arm64).

## Context
`Net.listen` and `Net.dial` returned a `Listener` and a `Conn`, but
neither had methods besides `Listener.addr()`: a listener could only be
handed to `http.server`. The native HTTP server is built bottom up on
the native event loop and needs a byte stream to test against both
backends; a TCP echo server is the smallest program that exercises
listen, accept, read, write, and close.

## Decision
Add to the prelude (std.md 3.2):

```
fn (l: Listener) accept() -> Conn ! NetErr     uses net.listen
fn (l: Listener) close()
fn (c: Conn) read(max: Int) -> Bytes? ! NetErr uses net
fn (c: Conn) write(data: Bytes) -> () ! NetErr uses net
fn (c: Conn) close()
```

- `read` returns at most `max` bytes, capped at 1 MiB (so `max` is not
  an allocation size), fewer when fewer are available, and `None` at the
  end of the stream; `max <= 0` gives empty Bytes. `write` writes every
  byte.
- `accept`, `read`, and `write` are cancellation points that report
  `NetErr.Canceled` (core 7.2), like `listen` and `dial`. `close` is
  idempotent and fails a waiting operation on the same object with
  `Io` ("use of closed network connection").
- Effects: `accept` is `net.listen` (it extends the listener's
  authority); `read` and `write` are `net`, since a Conn comes from
  either `dial` or `accept`. `close` performs no effect.
- Go backend: blocking calls on `net.Conn` and `net.Listener`, ended
  on cancellation by setting the deadline to the past; recorded and
  replayed as effects `net.accept`, `net.read`, `net.write`.

## Consequences
- An echo server and its clients run in one program on every backend,
  printing the same (`net_echo`).
- Error messages inside `Io` differ between backends (Go's error text
  and the native runtime's); outcomes (the variant) match.
- A replayed run has no sockets: replayed `read` and `write` give the
  recorded results.

## Alternatives
- **No effect on Conn methods** (the Conn as the authority, like an
  open file): simpler signatures, but a pure function could then do
  network I/O on a Conn it was handed, against core 6.1.
- **`read` returning empty Bytes at the end**: one less Option, but
  `Stdio.read_line` already uses `None` for the end of input.
