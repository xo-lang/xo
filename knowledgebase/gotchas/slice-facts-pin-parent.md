# A list of slices keeps whole parents alive

Status: Observed (2026-11-01)

Symptom: a phase that drops a large string per task still peaks as if it kept
them all. The self build's counter peak was hollow's, 4440 MB against emit's
3985, and dropping each hollowed text after its object moved it 43 MB.

Cause: a Str or Bytes slice is a view holding a count on its parent (decision
0330). `undef_scan` kept its symbol names as slices, so one short name kept a
whole part's hollowed text (993 MB live across the parts at the 80th).

Fix: `.clone()` what outlives the parent. Find it with a heap profile: the live
bytes are charged to the parent's allocation site, not the slice's.
