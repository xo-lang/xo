# std/testing changes

## 2026-10-20: feature: FakeNet raw connections

- Kind: feature
- Commit: (filled in after the push)
- Found by: survey (raw protocol drivers could not be unit tested: std/sql's Postgres driver, std/smtp)
- Go reference: net.Pipe and net/http/httptest's in process listeners (the idea; the buffering is Xo's)
- Test: internal/gen/testdata/golden/test_fakenet_raw (Go, arm64, LLVM; cmd/xo TestNoGCTests), std/rt TestMemConn

Decision 0292: a test's `os.net.dial(addr)` reaches the in memory
listener at `addr` (Refused without one) and `accept()` returns the other
end; `listen` takes host names and refuses a busy address; 64 KiB per
direction with task waits; close ends the peer's stream and fails its
writes; closing the listener resets its backlog; `tls.client` passes the
bytes through. Before, a dial in a test always failed with Refused.
