# Test temporary directories leak when a run is killed

Status: Observed (2026-10-10)

Symptom: the disk filled up. `os.TempDir()` held 409 `xo-*` directories
(8 GB, up to 2.8 GB each for `xo-gen-test-*`) from 2026-10-07 on, next
to an 85 GB Go build cache (`~/Library/Caches/go-build`) and a 33 GB
`~/Library/Caches/xo`.

Cause: two leaks.

- The `TestMain` of `internal/gen`, `internal/llvm` and `internal/arm64`
  removes its directory after `m.Run` returns, but a binary stopped by
  `-timeout`, a signal (an agent's command timeout), or `os.Exit` in a
  test never reaches that line.
- The http-diff driver in `internal/llvm/http_diff_test.go` made an
  `xo-http-diff-*` directory on every run and never removed it (about
  380 of them).

Fix: `TestMain` gets its directory from `testtmp.Dir(prefix)`
(`internal/testtmp`), which writes the owner PID into it and first
removes the `prefix*` directories whose owner has exited (or, with no
owner file, that are over 24 hours old). Concurrent runs from other
sessions keep their directories. The http-diff driver builds into
`testDir/http-diff`, and the llvm `TestMain` sweeps the old
`xo-http-diff-*` directories. A new test package with a shared temporary
directory should use `testtmp.Dir`, not `os.MkdirTemp`.

The Go build cache is not cleaned by any of this: `go clean -cache`
clears it (the next build is slower, nothing else is lost).
