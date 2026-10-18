# Unix socket paths longer than 104 bytes fail on macOS

Status: Observed (2026-10-18; anticipated 2026-10-08).

Symptom: `xo serve --socket=<path>` or a test using `t.TempDir()` for the
socket fails with `bind: invalid argument`.

Cause: `sun_path` holds 104 bytes on macOS (108 on Linux). `t.TempDir()`
paths under `/var/folders/.../T/TestName.../001/` plus a file name can exceed
it.

Fix: keep socket paths short: `TestServe` uses `os.MkdirTemp("", "xs")` and
`s.sock`; the default `~/Library/Caches/xo/serve.sock` is well under the limit.

Observed 2026-10-18: the full internal/arm64 and internal/llvm suites with
TMPDIR set to an agent scratchpad (about 110 bytes deep) failed only
unix_kill, on every backend: the golden built `$TMPDIR/xo-unix-N/s.sock`.
internal/llvm/testdata/golden/unix_kill now falls back to `/tmp/xo-unix-N`
when the directory plus its longest socket name ("/other.sock") would
pass 103 bytes.
