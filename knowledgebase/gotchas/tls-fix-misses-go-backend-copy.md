# A std/tls fix in std/native/llrt misses the Go backend's copy

Status: Observed (2026-11-17, task #209)

Symptom: the full tier's TestTLSServerReplayDiff (internal/llvm) fails with
"arm64 differs from go" and "llvm differs from go" on a case where the
native backends match Go's crypto/tls and the `go` column does not (case
1142 of seed 11: illegal_parameter on the Go backend, decode_error native).

Cause: the TLS servers exist twice, std/native/llrt/tls13srv.xo (and
tls12srv.xo) for the native backends and std/rt/internal_tls13srv.go for
the Go backend. The replay diff takes the Go backend as its reference, so a
fix landed only in the native file (2026-11-16, point formats and
status_request checks) turned the Go backend into the odd one out.

Fix: port each server fix to both files in the same commit, and check a
replay difference against real crypto/tls before deciding which side is
wrong: feed the dumped flights (XO_TLSSRV_DIFF_DUMP) to a tls.Server over
net.Pipe (records after the first at version 0x0303).
