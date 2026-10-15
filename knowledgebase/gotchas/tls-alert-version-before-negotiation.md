# crypto/tls writes TLS 1.0 records before the version is agreed

Observed 2026-10-15 (tls12srv.xo and tls12.xo under xo-2b's server
differential fuzz, TestTLSServerReplayDiff).

Symptom: an alert sent before the ClientHello (server) or ServerHello
(client) is processed differs from Go's in its record version:
`15030300020232` from Xo, `15030100020232` from crypto/tls.

Cause: crypto/tls's writeRecordLocked uses VersionTLS10 while c.vers is
still 0, so every record it writes before negotiation (a decode error,
an unexpected message, unsupported versions, an oversized record) says
0x0301.

Fix: send alerts with 0x0301 until the version is agreed (have_vers),
0x0303 after. Related replay rules the same fuzz showed: a test op must
feed the peer's bytes as one stream (the handshake reads records until
it ends, the rest waits), and input that stops inside a record is
crypto/tls's "unexpected EOF", not "EOF".
