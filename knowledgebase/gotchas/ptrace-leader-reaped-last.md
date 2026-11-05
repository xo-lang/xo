# A traced program's leader is reaped only after its other threads

Status: Observed (2026-11-05, Xo Lang 2, decision 0253 stage 6)

## Symptom

`xo debug` on Linux hung at the end of a session whose program had made
threads (cmd/xo TestDebugTasks on the linux/arm64 offload host): xo sat
in `wait4(pid, __WALL)` for the killed program, which ps showed as
`[prog] <defunct>` with state `Zl` (a zombie leader with live threads).

## Cause

After SIGKILL, every traced thread reports its exit to the tracer, and
the kernel holds the thread group's leader back (it cannot be reaped)
until each of those threads has been waited for. Waiting only for the
leader's pid never returns. Waiting for the table's tids first misses
them too: SIGKILL is asynchronous (they had not exited yet), and a thread
the program made just before the stop may not be in the table. A
`wait4(-1, __WALL)` loop until ECHILD is no better: xo has a child of its
own that lives on (the xo-lifeline helper), so ECHILD never comes.

## Fix

std/native/llrt/dbglinux.xo `reap` waits with `wait4(-1, __WALL)`,
resumes any stop on the way out, and ends when the leader's own pid is
returned. The exit path of `wait`, which has reaped the leader already
(it comes last), does not wait again.
