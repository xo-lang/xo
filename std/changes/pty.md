# std/pty changes

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

## 2026-11-22: feature: std/pty, pseudo-terminals and a child with a controlling terminal

- Kind: feature
- Commit: this commit
- Found by: task #691 (xw's password login needs ssh to read from a terminal the program holds), decision 0740
- Go reference: github.com/creack/pty v1.1.24 (Open, Start, StartWithSize, Getsize, Setsize, InheritSize) and golang.org/x/term (MakeRaw, Restore)
- Test: std/pty_test.xo (creack's open, start, read and write, size, and controlling-terminal tests, plus ControllingOnly, raw mode, deadlines, output written before the first read, and the refusals), `xo test --real --std-module=pty`; std/rt's TestPtyOutputBeforeFirstRead for the Go backend
- Shipped in: not yet released

`pty.open` opens a master and slave pair, `pty.spawn` and `pty.start` run a child in a new session with the slave as its controlling terminal (on its standard streams too with `Attach.All`, or only as /dev/tty with `Attach.ControllingOnly`), and the Pty reads, writes, resizes and switches raw mode. Linux, macOS and FreeBSD, native and Go backends; Windows and wasm return an `Io` error from `open`.
