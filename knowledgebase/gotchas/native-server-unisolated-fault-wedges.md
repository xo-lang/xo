# A fault outside a handler's isolation stops the native HTTP server

Status: Observed (2026-10-15, TestHTTPFuzz)

Symptom: after `GET /status/\x81204` (a handler slicing the path inside
a character), a native std/http server answered nothing more, to anyone:
the process alive, every thread parked (the listener in
poll.pd_wait_ctx, the poller in kevent, the workers in futex_wait), new
connections accepted by the kernel and never read. The Go backend
dropped that one connection (net/http's recover) and kept serving.

Cause: the fault was in a middleware (`r.wrap`), which ran outside the
task.isolate that std/native/http.xo puts around a route's handler; it
reached the connection's task, and the server stopped serving.

Fix: http.xo call_raw (and std/rt/http.go call) isolate the whole
middleware chain as a handler (500, logged). Open: a fault in the
connection's own code (std/http's, not the program's) would still stop
the server; the connection task should end that connection only.
Replay a fuzz input with XO_FUZZ_REPLAY (internal/llvm TestHTTPFuzz).
