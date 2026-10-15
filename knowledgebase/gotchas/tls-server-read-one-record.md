# A TLS server read that returns every buffered record hides HTTP/2's rapid reset

Status: Observed (2026-10-15, B31 server wiring, tlssrv.xo)

Symptom: with the HTTP server's TLS in Xo (tlssrv.xo), TestH2Diff's
"raw rapid reset" case left the connection open where Go and the Mbed
TLS path send GOAWAY ENHANCE_YOUR_CALM. 1,300 HEADERS each followed by
RST_STREAM, one TLS record per frame, never tripped h2srv.xo's limit.
It happened every run.

Cause: the first tlssrv.xo read decrypted every complete record it had
read from the socket (up to 16 KiB at a time) and handed the plaintext
to h2srv.xo in one piece. h2srv.xo then processed each HEADERS and the
RST_STREAM that cancels it in the same batch, without scheduling a
handler in between, so the queue of waiting handlers that the limit
counts never grew. Mbed TLS's mbedtls_ssl_read returns one record per
call, and so does Go's tls.Conn, so the frames arrive one record at a
time with the connection's loop running between them.

Fix: after the handshake, tlssrv.xo keeps the bytes read in Conn.pend
and gives tls13srv.xo (or tls12srv.xo) one complete record per read.
During the handshake it feeds everything at once. A TLS layer that
batches reads changes the HTTP/2 server's behavior under a flood; keep
one record per read.
