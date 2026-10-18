# TestH2Diff: the Go reference answers fewer of 400 streams under load

Status: Observed, fixed (2026-10-18, Xo Lang 3's gate of 3eea92b1)

Symptom: TestH2Diff failed with `go 400 streams: ok=378` on the Go
backend (the reference) against `ok=400` on LLVM and arm64, line 63 of
the observations. The same commit passed TestH2Diff run alone (53 s).

Cause: the case sends 400 requests at once on one connection, over the
server's limit of 250 concurrent streams; net/http's client queues the
rest, and under heavy load (TestTLSInteropMatrix, TestTLSServerReplayDiff,
TestNistecDiff, and TestX25519Diff in the same `go test -run`, each
starting servers) some of the Go reference server's streams time out
before they are served. The native servers answered all of them.

Fix: rerun TestH2Diff alone before calling a native change at fault;
run it apart from the TLS diff tests in gate scripts. Seen again
2026-10-18 in a gate that ran it as its own `go test` (ok=386): the
machine's load average was about 36 from other sessions' work; it
passed alone at a load of 6.

Fixed (2026-10-18, Xo Lang 3): seen a third time with the native server
short (`ok=381` of 400, load average 55), so neither side is at fault:
the case's client had a 20 s deadline and dropped its errors. It now
waits 2 * h2EventsMax (60 s) and logs each failure with t.Logf (outside
the compared output); 4 of 4 runs passed at a load of 50. The same gate
found the stop-with-a-stream-in-flight case racing: it sent HEADERS,
slept 50 ms and stopped the server, which answered `GOAWAY last=0` when
it had not read the stream yet. It now sends a PING after the HEADERS and
waits for the acknowledgment (h2conn.sync), which the server sends only
after reading the frames before it.

Related: h2diff-window-case-flaky-under-load.md (another TestH2Diff case
that timing under load changes).
