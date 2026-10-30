# 0445. `use go`: a []byte Go writes into binds as a length in, the filled Bytes out

Status: Accepted (owner, 2026-10-30). Previously proposed (Xo lang 5, 2026-10-30)

## Context

`use go` skipped every function with a `[]byte` parameter Go writes into
(the binder's outBuffer rule, added for the Windows tester's bug 18 on
2026-10-29): a Bytes is a value, so Go's writes into the copy were lost.
That skipped io.Reader's `Read` on every type (net connections, os.File,
bufio, gzip, strings and bytes readers), `ReadAt`, `io.ReadFull`,
`io.ReadAtLeast`, `crypto/rand.Read`, and hex's and base64's `Encode` and
`Decode`. In the tester's 87 package survey (bench/results
usego-coverage-2026-10-29b.md, reproduced on main at 82a509844 with Go
1.26.1) it was the largest skip reason in std that is not a decision to
keep a package unbound: 50 of std's 204 skipped functions and methods,
next to Go's slices and maps (50, decision 0415) and pointers Go writes
through (49, kept skipped by decision 0419).

Decision 0416 already binds the reverse direction: an Xo implementation
of `Read(p []byte) (int, error)` is `fn(Int) -> Bytes ! go.Error`.

## Decision

1. A function with one output buffer (outBuffer) takes an `Int` in its
   place, named `max`: the buffer's length. The adapter makes the buffer
   (`make([]byte, max)`; a negative length is a fault) and passes it.
2. The bytes Go wrote replace the leading `n int` result:
   `buf[:n]`. With no results (base64's `Encoding.Encode`), the whole
   buffer is the result. Other results stay (`ReadFrom`'s address:
   `read_from(max) -> (Bytes, Addr) ! go.Error`).
3. io.Reader's contract lets a read return n > 0 bytes with an error.
   With `io.EOF` the bytes are returned and the EOF is left for the next
   call, which gives it again (the contract's "subsequent calls"). Any
   other error is returned as the error.
4. Still skipped: two output buffers in one function (`ReadMsgUDP`'s b
   and oob), and an output buffer whose function's first result is not
   an `int` n.

## Consequences

- Reading from a Go reader works from Xo: `conn.read(4096)?`,
  `io.read_full(r, 16)?`, `hex.decode(hex.decoded_len(n), src)?`.
- Each call allocates its buffer; a program reading in a loop allocates
  per read (Go reuses one). Bytes that came with a non EOF error are not
  returned.
- Binding format 32. The survey (bench/results usego-coverage-2026-10-30.md): std 90.3% to
  92.5% (1898 to 1944 of 2102 functions and methods), third party 98.6% to 98.7%.

## Alternatives

- **A mutable buffer handle** (`go.Buf`, a Go `[]byte` with get/len):
  keeps Go's reuse but adds a type and a step to every read.
- **Keep skipping**: io.Reader, the most used Go interface, stays
  unreadable from Xo.
