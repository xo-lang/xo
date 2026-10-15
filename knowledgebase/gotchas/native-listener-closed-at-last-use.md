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
last use is not.
