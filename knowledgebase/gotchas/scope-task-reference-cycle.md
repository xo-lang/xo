# A failed task and its scope kept each other alive

Status: Observed (2026-10-08, std/llrt/testdata/tasks.c, decision 0068)

Symptom: the C test of the task runtime reported `failure: 3 live of
23656 allocations` after the child failure test; every other test freed
everything.

Cause: a task held a counted reference to its scope, and a scope whose
child failed keeps a reference to that task (its error is read after
the join). The scope's last handle went away, but its count stayed at
one (the task's), and the task's at one (the scope's): a cycle that
reference counting never frees (scope, task, result list).

Fix: a task's pointer to its scope is not counted. It is safe because
only a running task and the worker that ran it use it, and the scope's
join waits until every task finished and every queue entry of its tasks
was taken (see `xo_scope_close`). Runtime objects that point at each
other need the same care: decide which side owns, and make the other a
plain pointer whose lifetime a join or a lock guarantees.
