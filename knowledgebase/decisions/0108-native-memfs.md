# 0108: MemFs and os.proc.exit in run_main for native test binaries

Status: Accepted (2026-10-09). Implemented in `std/native/testing.xo`
(`memfs_*`), `std/native/rttest.xo` and `std/llrt/xo_testing.c`
(`test_memfs_take`, `test_memfs_put`, `test_now`, `test_glob`, `tw_exit`,
`xo_test_exit_code`, `xo_test_main_end`), `internal/ssa/build_io.go`
(`memFsCall`, `Proc.exit` in tests). Tests: cmd/xo
`testdata/nogc/same/fakes_test.xo` and the internal/gen golden
`test_fakes`, both in TestNoGCTests (report identical to the Go
backend's), TestNoGCTestLeaks.

## Context

Native test binaries had an empty, read only MemFs: every other Fs method
of a test was XO1201, and so were `fail_next`, `set_case_sensitive`, and
`files`. `os.proc.exit` inside `testing.run_main` failed the test with
"not available".

## Decision

1. **MemFs is std/rt's memFs and memView ported to Xo.** The file system
   is an Xo value (`MemState`: nodes by key, case sensitivity, pending
   failures, the temp file counter) that the C test world keeps as an
   `Any` between operations. An operation takes it (which locks the
   store), works on it without waiting, and puts it back (unlocking), so
   operations are atomic as under the Go backend's mutex. A view is the
   lowered Fs value itself (decision 0102): its root relative to the file
   system's root and its mode bits (sub, read only, write only).
2. In a test binary every Fs method, of `MemFs` or of a prelude `Fs` (a
   `sub` view, an Fs passed to the code under test), lowers to
   `memfs_<name>`. Modified times are the virtual clock's. `fail_next`
   patterns match with Go's `path.Match` (ported to C, a bad pattern
   matches nothing).
3. **`os.proc.exit` inside `run_main`** records the code, cancels main's
   root node, and returns with the context unwinding; generated code
   checks after the call in test binaries, so main unwinds (running its
   deferred code) back to `run_main`, which returns the code.
   `FakeProc.exit_code()` reports it. Outside `run_main` the test ends and
   passes, as before.

## Consequences

- Each operation copies the node map once (the state is shared with the
  stored value while the operation runs): linear in the number of files,
  which is small in tests.
- The test_fakes golden runs natively with the Go backend's report.
- A fault inside a MemFs operation would leave the store locked; the
  test process ends with the fault anyway.

## Alternatives

- **MemFs in C**: a second implementation of the path and error rules in
  C; the Xo port is a line by line reading of std/rt.
- **Redirecting the real Fs system calls to an in memory tree**: the
  real Fs's errors (Go's os package) differ from MemFs's (`write` creates
  parent directories, `remove` of the root), so the rules would split
  across two layers.
