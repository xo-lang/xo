# Leaks show up only on Linux (LeakSanitizer)

Status: Observed (2026-10-09)

Symptom: TestHTTPParseDiff failed on every Linux machine (EC2 arm64 and
amd64, Docker arm64) with `LeakSanitizer: detected memory leaks`, 256
bytes and up from `nbuf_grow` in `hw_response` called by `xo_hc_respond`
from `serve` in `std/llrt/testdata/http_diff.c`. It passed on macOS.

Cause: commit 17fb10f (2026-10-09, after the Docker Linux validation)
made the response buffer `hc->out` live for the connection; the runtime
frees it when a connection closes (`xo_http.c`), but the test driver's
`serve` built its own `xo_hconn` and never freed `hc.out.p`. A leak in
the test driver, not the runtime. AddressSanitizer on macOS arm64 has no
leak detection, so only Linux reports it.

Fix: `free(hc.out.p)` in `serve`. A C test driver that builds runtime
structs by hand must free every field the runtime's own destructor frees;
check against the destructor when adding a field. Run the Linux suite
after any llrt allocation change.
