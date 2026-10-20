# 0280: Connection effects by origin

Status: Accepted (owner, 2026-10-20): option (a) of decision 0278's
question, with the accepted connection named `ServerConn` and a prelude
`Stream` interface. Implemented by Xo Lang Lib 1 the same day.

## Context

`Net.dial` performs `net.dial` and `Listener.accept` performs
`net.listen`, but `Conn.read` and `Conn.write` performed `net`, the
union of both. So any code that reads a connection it dialed had to
declare `uses net`, which says it may also listen. std/sql's operations
declare `uses net.dial`, and a Postgres driver written in Xo (decision
0278) reads and writes its connection, so it performed more than its
interface allowed (the prototype hit XO0701). The same holds for any
client library written in Xo over `Net` (Redis, SMTP), and for the HTTP
client if it were written over the prelude's `Conn`.

## Decision

1. `Conn` is the connection `Net.dial` makes; its `read` and `write` are
   `uses net.dial`.
2. `Listener.accept` returns a new handle type, `ServerConn`, with the
   same three methods; its `read` and `write` are `uses net.listen`.
3. A prelude interface `Stream` (`read` and `write` under `net`, and
   `close`) is satisfied by both, for code that handles either kind
   (an echo loop, a line reader shared by a client and a server).
4. Both kinds are the same value at run time: the compiler lowers
   `ServerConn.read`, `write`, and `close` to the runtime calls of
   `Conn`'s (compiler/ssa conc.xo), the Go backend's `rt.ServerConn` is
   an alias of `rt.Conn`, and FakeNet and record and replay need no
   change. Programs that do not name the types compile to the same code.

## Consequences

- A client library declares `uses net.dial` and nothing more; a function
  given only a `ServerConn` cannot dial, so the effect says which side of
  the network a function works on.
- Code that names `Conn` for an accepted connection no longer checks
  (`expected Conn, found ServerConn`): in the repository that was xo
  serve (compiler/cli/servers/serve.xo, its line reader now takes a
  `Stream`), std/native/llrt hserve.xo's `hc_new`, and two test
  programs. Allowed in the `preview` edition (decision 0155).
- Conn's TLS upgrade (spec 3.2, Xo Lang 3) is dial side and so
  `net.dial`.

## Alternatives

- **A checker rule tracking each `Conn` value's origin.** One type with
  an effect carried by the value would need flow typing the checker does
  not have; two types say the same thing with the existing rules.
- **std/sql declares `uses net`.** Widens every caller's declaration to
  allow listening, which a database client never does.
- **Exempt std's driver.** Third party drivers would still need `net`.
- **The names `Peer`, or `DialConn` and `ListenConn`.** The owner chose
  `ServerConn`, keeping `Conn` for the common client case.
