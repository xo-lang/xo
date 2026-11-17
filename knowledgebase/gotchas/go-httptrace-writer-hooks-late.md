# Go's httptrace write hooks and connection reuse depend on the writer goroutine's timing

Status: Observed (2026-11-16, task #151)

Symptom: TestGoldenGoParity/http_client_trace and http_max_idle failed in
the 6 h tier on a loaded host and passed alone. With every CPU busy,
http_client_trace's "https: reused" exchange lacked WroteHeaders and
WroteRequest, and http_max_idle's second round of 8 dialed a ninth
connection.

Cause: net/http runs the WroteHeaders and WroteRequest hooks on the
connection's writer goroutine (http2 writeRequest, HTTP/1 writeLoop),
and RoundTrip returns on the response without waiting for them. HTTP/1
also keeps a connection only when the writer reports the request written
within 50 ms of the response (maxWriteWaitBeforeConnReuse). A starved
writer goroutine misses both. The native backends write on the calling
task, so they never see either.

Fix: std/rt's traceRec.deliver waits (at most a second) for WroteRequest
once GotConn was recorded. Goldens that count reused connections or print
event order on the Go backend run the timing-dependent part again (up to
5 times) when the pool dropped a connection or the write events came
after the response; a pool that never reuses still fails every run.
Reproduce with `XO_TEST_GO_PARITY_LOAD=20 go test ./internal/llvm -run
TestGoParityUnderLoad` (busy goroutines on every CPU), on an offload host.
