# A reader task applies the reply between two lock sections of the caller

Status: Observed (2026-11-06)

## Symptom

llvm TestGoldenTasks/amqptest_broker hung in about 1 run of 4 on
linux/arm64 (one test timeout of 30 minutes per hang), and once in 600
goldens on the linux/amd64 hourly run. Alone, the golden never hung (300
native runs and 8 TSan runs on an idle host); 16 copies at once pinned to
4 CPUs hung 3 times in the first 48. Every task was parked, the poller
waited with no timeout, and every TCP queue was empty: nothing was lost
on the wire.

## Cause

std/native/amqp.xo's `Sender.send` wrote the transfer's last frame in
one `ConnState` lock section (`step_send_chunk`) and recorded the
delivery as unsettled in a later one (`st_send_done`). With the reader
task running (`keepalive`), the broker's disposition could arrive and be
applied in between: it matched no unsettled delivery and was dropped,
and `send` then waited for an outcome that had already come. It needs a
loaded machine for the reader to win the lock before the caller's next
section.

## Fix

Record whatever the reply will be matched against in the same lock
section as the write that can cause the reply (the delivery now counts
as sent with its last frame). In any code with a reader task that
applies replies under a lock, check every request whose bookkeeping is
done after the lock that wrote it.

## How it was found

`xo debug -p PID --batch -c tasks` on the hung native build showed
which tasks were parked where, `/proc/PID/task/*/syscall` showed the
poller's `epoll_wait` with timeout -1, and `/proc/PID/net/tcp` showed
empty queues. Neither the stacks nor the locals settled it (xo debug
prints a Bytes or List local as raw words); a local branch tracing each
frame read, applied and written on both sides to stderr showed the
disposition applied with no waiter before `send` began to wait.
