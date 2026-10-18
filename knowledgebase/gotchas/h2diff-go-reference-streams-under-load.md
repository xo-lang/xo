# TestH2Diff: the Go reference answers fewer of 400 streams under load

Status: Observed (2026-10-18, Xo Lang 3's gate of 3eea92b1)

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
run it apart from the TLS diff tests in gate scripts.

Related: h2diff-window-case-flaky-under-load.md (another TestH2Diff case
that timing under load changes).
