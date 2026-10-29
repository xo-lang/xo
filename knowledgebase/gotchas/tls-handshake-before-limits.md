# A server's TLS handshake runs before its limits reach the connection

Status: Observed (2026-10-30, xo-lang-6, HTTP server gaps part 3).

## Symptom

A golden that connects to an HTTPS server with a head timeout and sends
nothing hung for ever while `xo dev new-golden` recorded it (no timeout
of its own), which held a gate slot 24 minutes and the machine's queue
behind it. The first draft gave TLS reads the server's deadlines in
hconn.recv_until only, and the native http tests passed.

## Cause

serve_conn (std/native/http.xo) runs the TLS handshake first
(hserve.hc_h2, cabi_tls srvtls_h2, tlssrv.handshake) and stores the
connection's limits after it (http1.set_limits), so a deadline set
around the reads never reaches the handshake: a client that never sends
its ClientHello blocks the connection's task in the handshake's wait,
and a test client waiting for the server to close waits with it.

## Fix

hc_h2 takes the handshake's deadline (http.xo handshake_deadline: the
smaller positive of the read and head timeouts, net/http's
tlsHandshakeTimeout) and sets it on the TLS connection around the
handshake; past it the handshake fails and the connection closes
without a reply. Golden http_server_limits covers the silent TLS
client. When adding a deadline to a server path, check where the
connection's state is set up relative to the first wait, and run a new
golden with a time limit (`timeout 120 xo run`) before recording it.
