# std/pty changes

## 2026-11-22: fix: tasks opening and closing ptys at once no longer close each other's descriptors (native backend)

- Kind: fix
- Commit: this commit
- Found by: task #705, the stress program (std/evidence/pty/stress)
- Go reference: none (creack/pty closes an os.File once)
- Test: golden pty_tasks_descriptors (8 tasks, 1000 opens each: failed with "open pty: bad file descriptor" in 2 of 3 runs, passes 3 of 3 now)
- Shipped in: not yet released

`Pty.close` closed the master's descriptor twice: the poller's record closed it, then close closed the number again, and a pty, pipe or file another task had opened under that number in between was closed under it. Its next call failed (`bad file descriptor`, `device not configured`, `fork/exec: operation not permitted`) or a spawn waited forever. Also on macOS an open of /dev/ptmx that the kernel fails with errno -6 under churn (about 1 in 3000, a Go program sees it too) is tried again (gotchas/pty-darwin-ptmx-negative-errno.md).

## 2026-11-22: fix: a read's max error says the same on the Go backend

- Kind: fix
- Commit: this commit
- Found by: task #705, the feature audit (std/evidence/pty/features.md)
- Go reference: none (Xo's own bound)
- Test: std/rt TestPtyReadMaxMessage (failed with "read: max is 0 to 1 MiB", passes now); std/pty_test.xo "a read's max is 0 to 1 MiB"
- Shipped in: not yet released

A `read` with a max outside 0 to 1048576 is an `Io` error reading `read: max N is not 0 to 1048576` on every backend; the Go backend said `read: max is 0 to 1 MiB`. New tests cover a canceled task's read (Canceled), kill_group reaching the child's session, and inherit_size, make_raw and restore on a program's own terminal (golden pty_own_terminal).
## 2026-11-22: feature: std/pty on Windows on the Go backend

- Kind: feature
- Commit: this commit
- Found by: task #704, decision 0740 stage 2
- Go reference: none (creack/pty has no Windows pty); os/exec's command line and environment block rules
- Test: std/rt TestPtyWindowsOutputThenEnd, ExitCode, Write, Size, ControllingOnly, ReadDeadline: 6 pass on Windows amd64
- Shipped in: not yet released

The Go backend's std/pty works on Windows as the native one does: a pseudo console, a child started with it by CreateProcessW (os/exec cannot attach one), reads with deadlines through a reader goroutine, and the pipe's end after the child's exit and 100 ms of quiet output.

## 2026-11-22: feature: std/pty on Windows over a ConPTY (native backend)

- Kind: feature
- Commit: this commit
- Found by: task #704, decision 0740 stage 2
- Go reference: github.com/creack/pty v1.1.24 has no Windows pty; the ConPTY calls follow Microsoft's CreatePseudoConsole documentation
- Test: a Windows amd64 check program (open, set_size then size, start with output then end, exit code, write reaching the child, start with a size, ControllingOnly, read deadline): 8 pass
- Shipped in: not yet released

`pty.open`, `pty.spawn` and `pty.start` work on Windows on the native backend: the Pty is a pseudo console (CreatePseudoConsole over two pipes), a child gets it with PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, and reads end after the child exits and its last output is out. The slave path is empty, `size` returns what was last set, and `set_raw` changes nothing. The Go backend on Windows still returns the `Io` error.

## 2026-11-22: fix: a close from another task ends a pending read on the Go backend

- Kind: fix
- Commit: this commit
- Found by: task #705, porting creack/pty v1.1.24's io_test.go TestReadClose
- Go reference: creack/pty TestReadClose (a close interrupts an outstanding read)
- Test: std/rt TestPtyCloseEndsRead (failed on darwin with "read: bad file descriptor", passes now); std/pty_test.xo "close ends a read waiting in another task"
- Shipped in: not yet released

On the Go backend, `read` waiting while another task closed the Pty returned an `Io` error (Linux: use of a closed file; macOS and FreeBSD: bad file descriptor, and the wait loop could poll a descriptor number reused by another file). It now returns none, the end, as the native backends do. std/pty_test.xo also ports creack's TestOpenByName, TestReadWriteText and TestReadWriteControls.

## 2026-11-22: feature: std/pty, pseudo-terminals and a child with a controlling terminal

- Kind: feature
- Commit: this commit
- Found by: task #691 (xw's password login needs ssh to read from a terminal the program holds), decision 0740
- Go reference: github.com/creack/pty v1.1.24 (Open, Start, StartWithSize, Getsize, Setsize, InheritSize) and golang.org/x/term (MakeRaw, Restore)
- Test: std/pty_test.xo (creack's open, start, read and write, size, and controlling-terminal tests, plus ControllingOnly, raw mode, deadlines, output written before the first read, and the refusals), `xo test --real --std-module=pty`; std/rt's TestPtyOutputBeforeFirstRead for the Go backend
- Shipped in: not yet released

`pty.open` opens a master and slave pair, `pty.spawn` and `pty.start` run a child in a new session with the slave as its controlling terminal (on its standard streams too with `Attach.All`, or only as /dev/tty with `Attach.ControllingOnly`), and the Pty reads, writes, resizes and switches raw mode. Linux, macOS and FreeBSD, native and Go backends; Windows and wasm return an `Io` error from `open`.
