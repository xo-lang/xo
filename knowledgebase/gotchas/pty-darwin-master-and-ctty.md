# Pseudo-terminals: the traps std/pty hit (macOS, Linux, FreeBSD)

Status: Observed (2026-11-22, task #691)

## Symptom

1. `Pty.size`, `set_size` and `set_raw` on a fresh master returned
   "inappropriate ioctl for device" (ENOTTY) on darwin.
2. A child started on the pty wrote its output, and a read after
   `Child.wait` returned the end with no data.
3. A child started with `Attach.ControllingOnly` was a session leader
   with no terminal (`ps` TTY `??`), and `echo x > /dev/tty` failed with
   "Device not configured".

## Cause

1. macOS answers TIOCGWINSZ, TIOCSWINSZ and termios ioctls on a ptmx
   master with ENOTTY until its slave has been opened once. The slave's
   next first open also resets the size and settings.
2. Once the child is reaped and no process holds the slave, macOS drops
   the bytes the master had not read yet. Linux keeps them.
3. macOS drops a session's controlling terminal at the last close of the
   descriptor TIOCSCTTY took. With the streams on pipes, the only slave
   descriptor in the child was close-on-exec.

## Fix

1. `ptyos.open` opens the slave right after the master and keeps it in
   the record (PT_SLAVE) until a spawn takes it (`take_slave`), as
   creack/pty's Open returns the tty open.
2. Read the master to its end before `wait` (the tests' `drain`).
3. `child_darwin` clears FD_CLOEXEC on the slave when it is not the
   child's standard input, as Go's Setctty wants Ctty among the child's
   descriptors.

Checked with python3's fcntl on the Mac first (notes/xo-4c/t691), then
by std/pty_test.xo (`xo test --real --std-module=pty`, 12 pass).

## Go backend: the poller never reports the master's end (Observed)

Symptom: on the Go backend on macOS, a read on the master opened with
os.OpenFile (and so registered with Go's kqueue poller) blocked forever
after the child exited; on Linux the same read returns EIO.

Cause: kqueue on darwin does not report a pty master as readable or at
EOF when the slave's last holder closes it, so the poller never wakes the
read.

Fix: std/rt/pty_bsd.go (darwin and FreeBSD) opens the master with syscall.Open (blocking,
outside the poller) and reads through a select(2) loop in 50 ms slices,
checking the task's cancellation and the read deadline between slices;
the read then returns EIO at the end. Linux keeps the poller
(std/rt/pty_linux.go). Checked by the t691/g1 program built with
`--backend=go` (2026-11-22, all checks ok).

## Linux: the same ctty trap, and Fd() turns the deadline off

Observed 2026-11-22 (task #691, host 2), after the macOS fixes passed:

1. Native, `Attach.ControllingOnly`: the master's first read returned
   the end before the child wrote "prompt" to /dev/tty. Linux reads EIO
   on a master while no descriptor holds the slave, and a controlling
   terminal alone is none; the slave fd closed at the exec (close on
   exec) and sh had not opened /dev/tty yet. Fix: proc.xo's Linux and
   FreeBSD child clears FD_CLOEXEC on ctty when it is not the child's
   stdin, as darwin's does. pty_test.xo's ControllingOnly test fails
   on host 2 with the old proc.xo (11 of 12) and passes with the fix.
2. Go backend: a read past its deadline blocked until the child's end.
   std/rt's ioctl helper called `f.Fd()`, which puts the master in
   blocking mode, and a blocking os.File ignores SetReadDeadline. Fix:
   the ioctls go through `f.SyscallConn().Control`. A probe
   (`--backend=go`, linux/arm64) read "end after 3008 ms" before and
   "timeout after 107 ms" after.

The gate does not catch either: stdfast skips pty's tests (they use
proc and clock, so they need `--real`). Run them on a linux host with
`xo test --real --std-module=pty` (a linux/arm64 xo and the tree, on host 2).

## FreeBSD (Go backend): a short child's output was gone

Observed 2026-11-22 (task #691, FreeBSD 14.5 amd64 VM). `stty -a` on a
pty read back empty in 6 of 40 runs, and in 40 of 40 when the parent
paused 100 ms before its first read; native was 0 of 120. A second read
found nothing either: the bytes were discarded, not late.

Cause, from truss on both backends: when a session leader exits, FreeBSD
revokes its controlling terminal, and the revoke discards the output the
master had not read if another descriptor still holds the slave. When the
leader's own close is the last one, the output stays readable. Native
closes its slave copy right after the exec-status pipe reports the exec,
before a short child can finish; Go's `exec.Cmd.Start` returns much
later, so the parent's copy (`defer s.Close()` in `Pty_spawn`) was often
still open at the child's exit. Holding the slave longer (until the child
exited and the master read dry) made it worse: 40 of 40 late, and g1's
raw and All checks failed. A second, smaller part: `os.OpenFile` puts the
slave in the poller, which sets O_NONBLOCK on the open file description the
child inherits; opening it blocking took the late case from 40 to 8 of 40.

Fix (std/rt): `ptyOpenSlave` opens the slave blocking (`syscall.Open`,
`os.NewFile`); on FreeBSD `ptyTraceExec` sets `SysProcAttr.Ptrace`, so the
child stops at its exec, and `ptyRelease` takes that stop with wait6(2)
(a WNOWAIT peek first, so a child killed meanwhile is left for Wait),
closes the parent's slave, then `PT_DETACH`es the child. Darwin and Linux
close the slave after Start as before. `TestPtyOutputBeforeFirstRead`
(std/rt) and std/pty's "a child that writes and exits before the first
read keeps its output" pause before the first read: the Go test fails on
the old std/rt on the VM (run 1: output "") and passes now; the probes
read 0 of 120 empty.
