# std/pty changes

## 2026-11-22: feature: std/pty, pseudo-terminals and a child with a controlling terminal

- Kind: feature
- Commit: this commit
- Found by: task #691 (xw's password login needs ssh to read from a terminal the program holds), decision 0740
- Go reference: github.com/creack/pty v1.1.24 (Open, Start, StartWithSize, Getsize, Setsize, InheritSize) and golang.org/x/term (MakeRaw, Restore)
- Test: std/pty_test.xo (creack's open, start, read and write, size, and controlling-terminal tests, plus ControllingOnly, raw mode, deadlines, output written before the first read, and the refusals), `xo test --real --std-module=pty`; std/rt's TestPtyOutputBeforeFirstRead for the Go backend
- Shipped in: not yet released

`pty.open` opens a master and slave pair, `pty.spawn` and `pty.start` run a child in a new session with the slave as its controlling terminal (on its standard streams too with `Attach.All`, or only as /dev/tty with `Attach.ControllingOnly`), and the Pty reads, writes, resizes and switches raw mode. Linux, macOS and FreeBSD, native and Go backends; Windows and wasm return an `Io` error from `open`.
