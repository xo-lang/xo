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

How to avoid: an edge added to quiet a race report in the runtime must not
join two tasks of the user's program; run `TestRaceSyncTable` (its
unsynchronized cases must still report) with the change, besides the
golden that showed the report.
