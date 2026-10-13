# A request body cut short: the native server closes, the Go backend does not

Status: Observed (2026-10-13, TestHTTP1ParseDiff)

Symptom: a request whose Content-Length promises 256 KiB or more than the
client sends before it closes its side (for example `Content-Length:
1000000` and a body of 2 bytes) gets `Connection: close` in the native
server's answer (xo_http.c's `too_big`), but none from the Go backend's
server. Plain net/http with the same body reading (io.ReadFull or
io.ReadAll, as std/rt's readBody) does close (checked with a scratch Go
server on 2026-10-13), so std/rt's server differs from net/http alone
there; TestHTTPParseDiff compares the C with plain net/http
(diffHandler), so it never saw this.

Cause: not found yet; the C copies net/http's `unreadDataSizeLocked() >=
maxPostHandlerReadBytes` rule, which the Go backend's server evidently
does not reach (a full duplex response or the order of its body read and
header write would explain it).

Fix: none yet; TestHTTP1ParseDiff leaves such inputs out (comment there).
Next: print `w.closeAfterReply` and the body's state from std/rt's
handler path in a scratch build of the Go backend's runtime to see which
branch net/http takes, then make the C follow the Go backend (the
reference) and move TestHTTPParseDiff's reference from diffHandler to
the Go backend's server.
