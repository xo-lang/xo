# macOS: /dev/ptmx opens fail with errno -6 under churn

Status: Observed 2026-11-22 (xo-4c, task #705).

## Symptom

With eight tasks opening and closing pseudo-terminals as fast as they
can, about 1 open of `/dev/ptmx` in 3000 fails, and its error reads
`open pty: unknown error: 4294967290`. A single task never sees it
(54585 opens in 10 s, none failed).

## Cause

The macOS kernel itself: `open("/dev/ptmx")` returns -1 with errno -6
(0xFFFFFFFA), which is no errno, while other threads open and close
ptys. A plain Go program (8 goroutines, `syscall.Open("/dev/ptmx")`,
grant, unlock, name, slave open, close) on the same Mac counted 38 of
105992 opens failing with `errno -6`, so it is not Xo's. The next
open works.

## Fix

std/native/llrt/ptyos.xo `open_master` opens again, at most 100 times,
while the result is that errno. Afterwards 81092 opens by 8 tasks and
90547 by 32 tasks in 10 s gave no error. The golden
internal/llvm/testdata/golden/pty_tasks_descriptors would be flaky
without it.

Found alongside a real Xo bug in the same churn: `ptyos.free` closed the
master twice (once through the poller's `pd_close`, then again), closing
what another task had opened under the number in between (changes log
std/changes/pty.md, 2026-11-22).
