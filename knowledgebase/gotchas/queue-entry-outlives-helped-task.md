# A task run by a helper is still in a queue

Status: Anticipated (2026-10-08, std/llrt task runtime, decision 0068;
designed around before it was hit)

Symptom (would be): a use after free in a worker, or a leak check that
reports a few live allocations at exit only sometimes.

Cause: `t.await` and a scope's join run a task nobody started on the
waiting thread (they claim it with a compare and swap of its state). The
task's entry stays in whichever deque it was pushed to until a worker
pops it and finds it already run. If the scope freed its tasks at the
join, that worker would read freed memory; if the queue held a counted
reference instead, the task could still be alive when `main` returns and
the leak check reads the counters.

Fix: a scope counts its tasks' queue entries (`queued`); a worker that
pops an entry decrements it after it ran the task or found it run
(`dequeued`), and the join waits until both `running` and `queued` are
zero. Queue entries hold no reference: the scope's list keeps each task
alive until the join ends.

Update (2026-10-09, decision 0077): `queued` and `running` are one atomic
word (`pending`, two per unit, bit 0 while the joiner waits); a task a
worker takes from a queue releases its entry and its run with one atomic
at its end, and a helped task's entry is still released by whoever takes
it (`dequeued`). The scope also unlinks its node of tasks before it frees
them, since task nodes now stay linked until the scope closes.
