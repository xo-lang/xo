# Unix socket paths longer than 104 bytes fail on macOS

Status: Anticipated (2026-10-08).

Symptom: `xo serve --socket=<path>` or a test using `t.TempDir()` for the
socket fails with `bind: invalid argument`.

Cause: `sun_path` holds 104 bytes on macOS (108 on Linux). `t.TempDir()`
paths under `/var/folders/.../T/TestName.../001/` plus a file name can exceed
it.

Fix: keep socket paths short: `TestServe` uses `os.MkdirTemp("", "xs")` and
`s.sock`; the default `~/Library/Caches/xo/serve.sock` is well under the limit.
