# A TSan release/acquire in the runtime can hide a race in the user's program

Status: Observed (2026-10-31)

Symptom: `TestRaceSyncTable` and `TestRaceSyncTableArm64` failed on main
with `mutex threads="1": no race reported without the synchronization`
(the unsynchronized form of the case must report a race).

Cause: 6fda98c55 gave the recycled task context a TSan release (at the
end of a run) and acquire (at the start of the next run) so the runtime's
own memset of the context did not race with a waker's lock write. TSan's
release/acquire carries the whole vector clock, not only the context: the
finished task's writes then happen before the next task, so two user tasks
that share an unsynchronized word look ordered.

Fix: no happens-before edge between runs. Under `--race` (ctx.SV_TSAN)
`run_claimed` keeps no spare context and frees it through the allocator;
1f5d91391's park lock take and drop orders the wakers before the free.

How it reached main: the check for 6fda98c55 and 1f5d91391 ran on
linux/arm64 and counted `WARNING: ThreadSanitizer` lines in the logs of
conc_reader_yield (x80) and TestRaceSyncTable (x10). That count cannot
see this failure: the test fails on the absence of a report, which leaves
no warning line to count. The 1f5d91391 gate log (h2kgate-cmd.log) shows
the fixed point only; TestRaceSyncTable was not run on the Mac.

Rule: a TSan annotation (tsan release/acquire, sanhook) or an llrt
tasks.xo or sched change runs `TestRaceSyncTable` and
`TestRaceSyncTableArm64` on the Mac in its gate, and the verdict is the
tests' pass or fail, not a count of reports.

How to avoid: an edge added to quiet a race report in the runtime must not
join two tasks of the user's program; run `TestRaceSyncTable` (its
unsynchronized cases must still report) with the change, besides the
golden that showed the report.

## Addendum (2026-10-31): the Linux TestRaceSyncTable red was a missing edge

Under --race a fiber switch orders nothing (sanhook.switch_flag 1), so the
worker thread's TSan context and the task's fiber share no clock. run_item
claims a task (a CAS on its T_STATE) in the worker's context, then the task
runs in its own fiber; the task's end (T_GONE release add) carries only the
fiber's clock, so the joiner that frees the task record after acquiring it
never saw the claim: TSan reported "free vs previous atomic write in
run_item" (chan, chan_buffered, select, scope cases; Linux only). Fix: a
sanhook.release(it + 4) after the claim (a mark of its own) and a
sanhook.acquire(t + 4) in task_free before the free. The first form,
release(it) taken by run_claimed's acquire(t), fixed Linux but gave the
task's run the worker context's whole clock and hid the table's racy cases
(chan, chan_buffered, select reported nothing without the synchronization,
Mac gate 2026-10-31): the edge must go to the freer, not the task.
Not the fix (tried, negative): an acquire of T_GONE before the free, a TSan
release/acquire around each run_fiber segment, keeping a spare context.
Lesson: a write in the worker's context that a fiber later frees needs an
explicit edge to the fiber that runs next.
