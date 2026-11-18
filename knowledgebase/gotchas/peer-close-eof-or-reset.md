# A peer that closes at once is EOF or "connection reset", by timing

Status: Observed (TestTLSDiff flake, reported 2026-10-10; fixed 2026-10-11)

Symptom: TestTLSDiff failed now and then on its `eof` line: one backend
printed `Protocol(Get "https://<eof>/x": EOF)`, another a "connection
reset by peer" error, for the same peer.

Cause: the `eof` peer closes every connection as soon as it accepts it.
When the close reaches the client before the client's ClientHello is
sent, the client's read sees EOF; when the ClientHello reaches the
closed socket first, the peer's kernel answers with a reset and the read
fails with ECONNRESET. Which comes first is a race between the two
processes, on every backend, Go included.

Fix: TestTLSDiff (internal/llvm/tls_diff_test.go, abruptClose) treats
both texts of the `eof` line as one result before comparing. Any test
of an abruptly closed connection must accept both, or make the close
deterministic (read the ClientHello first, then close).

## 2026-11-18: again in TestSmtpFuzz (task #318)

Seed 2 case 1333 on llvm (host 2): the fake server wrote the whole EHLO
reply and closed; the Xo client's AUTH reached the server before its
close, the close found it unread and sent a RST, and the client read
"connection reset" where Go's SendMail (slower to send) read EOF. arm64
matched Go on all 5000. Fix: the harness hangs up with smfHangUp
(CloseWrite, then read and drop until the client closes, 5 s bound), so
the client reads EOF however the command and the close race. Test:
TestSmfHangUpUnreadIsEOF (with a plain Close the read is "connection
reset by peer" every time). A fake server in a fuzz harness that closes
after a reply hangs up this way.
