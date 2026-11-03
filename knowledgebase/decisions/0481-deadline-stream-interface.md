# 0481: A connection interface with deadlines, beside Stream

Status: Accepted (owner, 2026-11-03; through the lead); implementation in
progress (Xo Lang 2). Supersedes the "Options for the owner" entry of
[0430](0430-connection-deadlines.md) (adding the setters to `Stream`).

## Context

Decision 0430 gave `Conn`, `ServerConn` and `TlsConn` Go's deadline
setters (`set_deadline`, `set_read_deadline`, `set_write_deadline`)
on the concrete types only. Its open option was to add them to the
`Stream` interface, which would break every user type that implements
Stream. The owner chose not to break Stream: a separate interface, as Go
keeps net.Conn beside io.ReadWriteCloser, carries the setters.

## Decision

A prelude interface, Go's net.Conn cut to what 0430 implements:

```
/// A connection whose reads and writes take deadlines (Go's net.Conn):
/// Conn, ServerConn and TlsConn (decision 0481).
interface NetConn {
  fn read(self, max: Int) -> Bytes? ! NetErr uses net
  fn write(self, data: Bytes) -> () ! NetErr uses net
  fn close(self)
  fn set_deadline(self, clock: Clock, t: Time?) uses clock
  fn set_read_deadline(self, clock: Clock, t: Time?) uses clock
  fn set_write_deadline(self, clock: Clock, t: Time?) uses clock
}
```

- Interfaces are satisfied structurally (core 2.4), so the three
  connection types satisfy it with no change, and every NetConn is also
  a Stream (a function taking Stream accepts one).
- `Stream` is unchanged; user types implementing it keep working.

### The join (the open check)

Core 2.10: without a known type, `if tls { tls_conn } else { conn }` does
not convert its arms and is `XO0401` (branches have different types),
with the new interface as with Stream. With the type written, each arm
converts: `let c: NetConn = if tls { tls_conn } else { conn }` checks.
So the user writes the type; the queueworker shape needs nothing more.
Joining two arms to a common interface without an annotation would be a
language change (which interface, when several fit) and is left to its
own proposal, not part of this decision.

## Consequences

- spec/std.md lists NetConn beside Stream; std/changes/prelude.md.
- A golden on the queueworker shape (dial plain or TLS, pick by a flag
  with the annotated `if`, set a read deadline through NetConn and see the
  read time out) on every backend.

## Alternatives

- Add the setters to Stream (0430's option): rejected by the owner, it
  breaks every user Stream.
- A join rule that picks a common interface: left to a separate
  language proposal (above).
