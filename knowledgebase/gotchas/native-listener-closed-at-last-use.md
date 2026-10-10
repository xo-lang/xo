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
