# std/pty feature audit (decision 0691)

Every feature spec/std.md section 91 and decision 0740 promise for std/pty, each with the test that holds it. The
module's API takes the shape of github.com/creack/pty v1.1.24 and golang.org/x/term's MakeRaw and Restore; no Go
package maps to it in scripts/stdaudit/map.txt, since creack/pty is outside Go's standard library. creack's tests are
ported in std/pty_test.xo (doc_test.go, io_test.go, run_test.go, start_test.go). Written 2026-11-22 (xo-4c, task #705).

- [x] open: a master and the slave's path under /dev, close twice does nothing | 0740 | std/pty_test.xo
- [x] the slave opened by its path reaches the master (creack TestOpenByName) | 0740 | std/pty_test.xo
- [x] start and spawn: the child on the terminal, its output read to the end, its exit status | 0740 | std/pty_test.xo
- [x] Attach.All: the child's standard streams and controlling terminal are the slave | 0740 | std/pty_test.xo
- [x] Attach.All refuses Cmd.stdin data and merge_stderr with ProcErr.Io | 0740 | std/pty_test.xo
- [x] Attach.ControllingOnly: the child's output on its pipe, /dev/tty on the terminal | 0740 | std/pty_test.xo
- [x] a missing command is an error and leaves the pty open | 0740 | std/pty_test.xo
- [x] size and set_size round trip, pixels included (creack TestGetsize, TestGetsizeFull, TestSetsize) | 0740 | std/pty_test.xo
- [x] start with a size: the child sees it before it runs | 0740 | std/pty_test.xo
- [x] write: typed input reaches the child; the echo maps LF to CR LF (creack TestReadWriteText) | 0740 | std/pty_test.xo
- [x] a control byte in a cooked line reaches the child and echoes as ^H (creack TestReadWriteControls) | 0740 | std/pty_test.xo
- [x] set_read_deadline: a read past it is Timeout, None clears it (creack TestReadDeadline) | 0740 | std/pty_test.xo
- [x] close from another task ends a waiting read (creack TestReadClose) | 0740 | std/rt/pty_late_test.go:TestPtyCloseEndsRead
- [x] set_raw(true) turns off echo and line editing; set_raw(false) restores cooked mode | 0740 | std/pty_test.xo
- [x] output written before the first read survives the child's exit (FreeBSD revoke) | 0740 | std/rt/pty_late_test.go:TestPtyOutputBeforeFirstRead
- [x] Windows: a ConPTY on both backends (empty slave path, size as set, set_raw a no-op) | 0740 stage 2 | std/rt/pty_windows_test.go
- [x] read: max over 1 MiB is an Io error, the same text on every backend | 0740 | std/pty_test.xo
- [x] a canceled task's read is Canceled | 0740 | std/pty_test.xo
- [x] inherit_size: copies this program's terminal size; Io when no standard stream is a terminal | 0740 | internal/llvm/testdata/golden/pty_own_terminal
- [x] make_raw and restore on this program's standard input; Io when it is not a terminal | 0740 | internal/llvm/testdata/golden/pty_own_terminal
- [x] spawn gives the child a new session, so Child.kill_group reaches it | 0740 | std/pty_test.xo
