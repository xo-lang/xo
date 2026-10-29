# A channel fed from outside the task graph faults as a deadlock

Status: Observed (2026-10-29, Xo Lang 2, Proc.notify on the Go backend)

## Symptom

A program that only waits on a channel the runtime itself feeds (signals
from Proc.notify; anything a goroutine, the poller thread, or a callback
sends into) dies with `fault: XO-F001: deadlock: every task is blocked
... on a channel made at FILE:LINE that no task can send on`, even though
the value would arrive. When the program sent itself the signal through a
child process, the run instead looked like it died of that signal, which
hid the cause.

## Cause

Both deadlock monitors (std/rt sched.go's monitor; natively tasks.xo's
park counting) count a task blocked in a channel receive as waiting on
another task. A sender outside the program's tasks (a goroutine of
std/rt, the native poller thread) is invisible to them, so once every task
waits, the program is reported deadlocked.

## Fix

Mark such a channel external, as task.stopping()'s channel is: on the Go
backend `c.ext = true` (std/rt chan.go), natively `CH_EXT` set in the
channel record (llrt/chan.xo; sigsub.xo subscribe). A wait on it is then
not counted. Test it with a program whose main waits on that channel alone
while the value comes later (golden signal_notify's late USR2).
