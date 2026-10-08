# Tests and a running `xo serve` share the default socket

Status: Anticipated (2026-10-08).

Symptom: `go test ./cmd/xo` results depend on whether a developer has
`xo serve` running, or `xo check` prints results that ignore a fix to the
checker.

Cause: `xo check` and `xo query` forward to whatever server answers on
`<user cache dir>/xo/serve.sock`. That server runs the code it was built
from, not the code under test or the binary just rebuilt.

Fix: `cmd/xo` tests set `XO_NO_SERVE=1` in `TestMain`; `TestServe` points
`XO_SERVE_SOCKET` at its own temporary socket. After rebuilding the
compiler, run `xo serve stop` and start it again (or set `XO_NO_SERVE=1`).
