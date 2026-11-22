# 0740: std/pty: pseudo-terminals and a child with a controlling terminal

Status: Accepted (owner 2026-11-22, xw decision #274; task #691, from
#263; xo-4c). Stage 1 (2026-11-22): macOS, Linux and FreeBSD on the native
and Go backends. Stage 2: Windows ConPTY, verified on the windows/arm64 VM;
until then Windows returns ProcErr.Io "pseudo-terminals are not supported
on this system", as wasm does.

## Context

xw's password login to hosts needs a child whose controlling terminal is
one the program holds (ssh reads a password from /dev/tty, not standard
input); later users are xo debug and tests of interactive programs. Xo can
spawn a child (decisions 0107, 0161, 0739) but cannot open a
pseudo-terminal pair or give a child a controlling terminal.

github.com/creack/pty (MIT) is the Go package for this: `pty.Open()`
returns the master and the slave, `pty.Start(cmd)` runs a command with the
slave on its three standard streams and as its controlling terminal
(`Setsid`, `Setctty`), `Getsize`, `Setsize` and `InheritSize` handle the
window size; raw mode lives in golang.org/x/term (`MakeRaw`, `Restore`).

Probes on the Mac (2026-11-22, C programs kept in the agent's notes):

- `posix_spawn` with `POSIX_SPAWN_SETSID` and a file action that opens the
  slave (with or without `O_NOCTTY`) does not give the child a
  controlling terminal: `ps -o tty=` prints `??` and opening `/dev/tty`
  fails with "Device not configured".
- `fork`, `setsid`, `open(slave, O_NOCTTY)`, `ioctl(slave, TIOCSCTTY, 0)`,
  then `exec`, with stdout and stderr dup'ed to a pipe: the child's tty is
  the slave, `/dev/tty` opens, stdout stays a pipe.

So on darwin a pty child cannot use llrt's `posix_spawn` path
(`spawn_darwin`); it needs the fork path linux already has
(`spawn_linux`).

## Decision

A std module `std/pty`, Xo's own API in creack/pty's shape:

```
use std/pty

struct Pty {}                       // the master side; a handle, copies share it
struct Size { rows: Int, cols: Int, x_pixels: Int = 0, y_pixels: Int = 0 }
enum Attach { All, ControllingOnly }

pub fn open(p: Proc) -> (Pty, Str) ! ProcErr uses proc   // the master and the slave's path
pub fn spawn(p: Pty, cmd: Cmd, attach: Attach = Attach.All, size: Size? = none) -> Child ! ProcErr uses proc
pub fn start(proc: Proc, cmd: Cmd, attach: Attach = Attach.All, size: Size? = none) -> (Pty, Child) ! ProcErr uses proc   // open, then spawn

fn (p: Pty) read(max: Int) -> Bytes? ! ProcErr uses proc    // none at end (the slave's last holder closed)
fn (p: Pty) write(data: Bytes) -> () ! ProcErr uses proc
fn (p: Pty) set_read_deadline(clock: Clock, t: Time?) uses clock
fn (p: Pty) size() -> Size ! ProcErr uses proc
fn (p: Pty) set_size(s: Size) -> () ! ProcErr uses proc
fn (p: Pty) set_raw(on: Bool) -> () ! ProcErr uses proc     // the slave's line discipline: raw or cooked
fn (p: Pty) close()

pub fn inherit_size(from: Stdio, to: Pty) -> () ! ProcErr uses (stdio, proc)
pub fn make_raw(io: Stdio) -> RawState ! ProcErr uses stdio // this program's own terminal
fn (s: RawState) restore() -> () ! ProcErr uses stdio
```

1. **`open`** is `posix_openpt(O_RDWR | O_NOCTTY)`, `grantpt`, `unlockpt`,
   `ptsname` on darwin, linux and freebsd; the master is close-on-exec.
   Windows opens a ConPTY (`CreatePseudoConsole` over two pipes); its
   slave path is empty.
2. **`spawn` with `Attach.All`** is `pty.Start`: the child gets a new
   session, the slave as its controlling terminal and on stdin, stdout and
   stderr; `Cmd.stdin(data)` and `merge_stderr` are refused (ProcErr.Io,
   "a pty child's standard streams are the terminal").
   **`Attach.ControllingOnly`** gives the child the new session and the
   controlling terminal but leaves its standard streams as the `Cmd`
   sets them (pipes by default): ssh reads the password from /dev/tty
   while xw reads its output with `Child.read`. Either way the `Child`
   works as today (pid, wait, kill; `kill_group` too, since a new session
   is a new group). Implementation: a new spawn flag bit (32, after
   0739's 16) and the slave fd passed to llrt `start()`; linux, freebsd
   and darwin take the fork path (`setsid`, `TIOCSCTTY`, then the
   existing dup2s); the Go backend sets `SysProcAttr{Setsid, Setctty,
   Ctty}`. Windows passes the ConPTY in
   `PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE`; with `ControllingOnly` the
   standard handles stay the `Cmd`'s pipes through `STARTF_USESTDHANDLES`
   (that a console reader such as ssh's password prompt still reads the
   ConPTY is to be verified on the windows/arm64 VM).
3. **Size**: `TIOCGWINSZ` and `TIOCSWINSZ`; Windows `ResizePseudoConsole`
   (rows and cols only; `size()` returns what was last set).
4. **Modes**: `set_raw` sets or clears raw mode on the pty with
   `tcgetattr` and `tcsetattr` (cfmakeraw's flags, as x/term's
   `MakeRaw`); `make_raw` and `restore` do the same for this program's
   own terminal, so a program can pass keystrokes through to a pty child.
   Windows: `set_raw` succeeds and does nothing (ConPTY has no line
   discipline the parent sets); `make_raw` sets the console mode.
5. **Capability**: `uses proc`, like spawning; no new capability. Wasm
   targets return ProcErr.Io "pseudo-terminals are not supported on this
   system".
6. **Tests**: creack/pty v1.1.24's tests ported with attribution (open,
   start, read and write through the pty, Getsize and Setsize, the
   controlling-terminal test), plus ControllingOnly: a child whose stdout
   is a pipe opens `/dev/tty`, reads a line written to the master, and
   echoes it to stdout. On darwin, linux and freebsd natively, the Go
   backend, and windows on the windows/arm64 VM.

## Consequences

- xw can log in with a password by giving ssh a controlling terminal it
  holds, without expect or sshpass.
- llrt's darwin spawn gains a fork path, used only for pty children;
  every other spawn keeps `posix_spawn`.
- spec/std.md gets a std/pty section, std/changes/pty.md its first entry,
  and the CHANGELOG a line, in the implementing change.

## Alternatives

- **Methods on `Cmd` (`Cmd.pty(p)`)**: reads well, but puts terminal
  handling in the prelude for one module's users; a std module keeps the
  prelude small and can grow (sessions, signals to the foreground group)
  without prelude changes.
- **Only `Attach.All` (creack's `Start`)**: xw's case needs ssh's output
  on a pipe apart from the terminal's echo and prompts; mixing them on
  the master would make xw parse the prompt stream.
- **`posix_spawn` on darwin**: the probe shows it does not set the
  controlling terminal.
- **An external helper (`script`, `expect`, sshpass)**: not on every
  host, and a password on a command line or in an environment leaks.
