# A TLS 1.3 server's alert can come after the client's handshake

Status: Observed (2026-10-09, decision 0106)

Symptom: against a Go server that requires a client certificate, the Go
client reported `Connect(remote error: tls: certificate required)` and
the native client `Protocol(Get "...": EOF)`. Recording alerts that
`mbedtls_ssl_read` returns fixed it on macOS and Linux arm64, but not on
Linux under x86-64 emulation, where the request's write failed first.

Cause: in TLS 1.3 the client finishes its handshake before the server
has checked the client's (empty) certificate, so the server's alert
arrives after the handshake, while the client sends its request. The
native client read nothing after its write failed (the server had
closed), so it never saw the alert; Go's transport reads and writes on
separate goroutines and reports the read error, a `net.OpError` (hence
`Connect`).

Fix: `std/llrt/xo_tls.c` records a fatal alert `mbedtls_ssl_read`
returns, and after a failed exchange without an HTTP level reason
`tls_alert` reads once more (100 ms at most) before the client maps the
failure; an alert becomes `Connect("remote error: tls: ...")`, also during
the handshake. TestTLSDiff's `clientcert` peer checks it.
