# Running a binary the test process just wrote: "text file busy"

Status: Observed (2026-10-14, linux/arm64, EC2 queue job tspeed-full-b)

Symptom: on Linux, the second (warm) `go test -short ./internal/llvm`
failed TestRuntimeRealtime with `fork/exec .../prog: text file busy`;
the first run, which compiled with clang, passed. The Mac never showed
it.

Cause: the C compile cache (XO_CC_CACHE, internal/xotool testcache.go)
copied the cached binary into place with Go file writes. The runtime
tests run in parallel, and another goroutine's fork while the copy's
descriptor was open gave the child a duplicate of it, held until the
child's exec; executing the binary in that window fails with ETXTBSY
(Go issue 22315). Writing then running an executable from a busy test
process is unsafe on Linux even after Close.

Fix: copy executables with `cp` in a child process (xotool copyExe), so
this process never holds a write descriptor on them; or have the tool
that makes the binary (clang, go build, xo) write it. Windows has no
ETXTBSY and copies in Go.
