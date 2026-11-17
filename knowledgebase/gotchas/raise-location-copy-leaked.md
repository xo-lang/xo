# A location copied for a raise leaked on every caught fault

Status: Observed 2026-11-17 (task #199).

Symptom: a long std/sse stress run grew its max RSS steadily while every
check passed. 100,000 Str slices that split a UTF-8 character inside
`task.isolate` grew max RSS by 6.2 MB; `fault(msg)` the same; an index fault
(which reports a position, not a copied location) did not grow.

Cause: `raise_str_at` and `raise` in std/native/llrt/fault.xo copied the
location bytes to the heap (`zcopy`) and passed the copy to `fire`, which
never returns. `fault.set` dups both the message and the location into the
fault context, so nothing ever freed the first copy. heap_large goes through
libc malloc, so `debug.mem_mapped` did not see it; `os.proc.usage().max_rss`
did.

Fix: `loc_after` writes the location into the per-thread message buffer right
after the message (unwind.msg_buf, MSG_BYTES) and copies to the heap only when
it does not fit. Guard: golden fault_isolate_leak. When a value is handed to a
function that never returns, check whether the callee copies it; if it does,
the caller's copy must live in a buffer that is reused, not on the heap.
