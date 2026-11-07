# A native Listener closes at its last use

Status: Observed (2026-10-09)

Symptom: a golden that listens on a port nobody accepts on (to make a
client wait for a handshake that never comes) printed `Timeout` on the Go
backend and `Connect(dial tcp ...: connect: connection refused)` on the
native backends.

Cause: the native backends free a value at its last use (reference
counting, decision 0046), and freeing a `Listener` closes its socket. The
program's last use of the listener was `l.addr()`, before the request, so
the port was closed when the client dialed. The Go backend's listener
stays open until the program ends (no finalizer closes it).

Fix: keep a listener alive by using it after the code that needs it
open, for example `l.close()` at the end. The same holds for `Conn`.

## In the runtime's own Xo: a handle's address taken at its last use

Observed 2026-10-14 (zero-C audit B28 part 3, std/native/llrt/hclient.xo).

Symptom: `xo mod tidy` through a module proxy failed with `Get "...":
EOF`; the client's send went to fd -1. lldb showed `conn_free` called
from inside `hclient.start`, right after the dial.

Cause: `let cna = rt.obj_addr(cn)` then `rc.inc(cna)` on a local `cn`
(the dial's result). The compiler releases `cn` at its last use, which is
the `obj_addr` call, so the release ran before the inlined `rc.inc` and a
Conn whose only reference that was got freed (its poll descriptor closed,
fd -1).

Fix: take the reference through a parameter, which the caller keeps alive
for the whole call: `fn retain(cn: Conn) -> Int { let a =
rt.obj_addr(cn); rc.inc(a); a }`. Adding to the count of a parameter
(`rc.inc(rt.obj_addr(param))`) is safe; doing it to a local after its
last use is not. Better still, keep the handle as an Int from an Xo
function that returns it owned: hclient.xo now dials with
`netconn.dial_timeout` (2026-10-15), whose Int result carries the
reference, so no typed local and no retain are involved.

Observed again 2026-10-15 (B31, tlsio.xo): the TLS connection's state
kept as `rt.any_of(conn)` in a local, then `rc.inc(rt.obj_addr(a))`. The
Any was freed at obj_addr, so the next call's `rt.any_get[Conn]` found
None (or garbage: a probe crashed with SIGBUS). The handshake finished,
then the request never went out and the server logged "TLS handshake
error: EOF". Fix as above: `fn keep(a: rt.Any) -> Int` takes the Any as
a parameter.

## A received connection kept only as `is_some()`

Observed 2026-11-07 (aws-19's FakeNet deadline repro, Xo Lang 1).

Symptom: a test accepts a `ServerConn`, sends it through a
`Chan[ServerConn]` (from a task or not), receives it as `let sc =
peer.recv()`, sets a read deadline on the client and reads: natively the
read returns `Ok(None)` (the end of the stream) instead of `Timeout`; the
Go backend gives `Timeout`. It looked like a use-after-free of the
connection the Chan carried; five variants (if-let or match, listener
kept, task kept alive, no task) all failed the same way, while every
variant that used the connection after the read passed.

Cause: the test's only later use was `expect(sc.is_some())`, which reads
the Option's tag, so the connection inside it had its last use at the
`recv`: it was released and closed there, before the read (std 3.2: a
connection dropped open is closed at the drop natively). The Chan and the
task were never involved. `let _sc = l.accept()...` does the same at
once (a write deadline test saw a reset peer instead of `Timeout`).

Fix: unwrap and use the connection after the code that needs its peer
open, for example `let sc = peer.recv().or_fault("recv")` and
`sc.close()` at the end.
