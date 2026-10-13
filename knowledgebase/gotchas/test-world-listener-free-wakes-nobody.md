# A freed in memory listener woke no waiting client (test world)

Status: Observed (2026-10-12, on Windows), fixed on main (mem-listener-backlog-after-serve.md).

Symptom: `xo test --no-gc --backend=llvm` of
internal/gen/testdata/golden/test_shutdown failed "a server finishes the
request in flight after the stop" with `XO-F001: deadlock: every task
is blocked and no timer is pending` in most runs on Windows with more
than one worker (1 of 10 with 2 workers, 0 of 10 with 4); always fine
with one worker, and on macOS (100 of 100).

Cause: a client request to an in memory listener that is not served
waits on the listener's waiter list until it is served or closed. A
server that stops releases its listener; the listener's free unlinked it
but woke nobody (only an explicit close did). When the free came after
the late client's last check, the client slept forever on memory that
was then freed, and nothing else was left to run. Windows' scheduling
made that order common.

Fix: the branch's own fix (one waiter list for every listener, woken by
the free) was dropped in the merge for main's (afe4aefb, 5626ecc7):
the listener records `served`, and its free wakes its waiters and keeps
the listener's memory until the last waiting client leaves
(mem-listener-backlog-after-serve.md). Both cover the free after a
waiter's last check. A wait on an object's own list must end before the
object can be freed.
