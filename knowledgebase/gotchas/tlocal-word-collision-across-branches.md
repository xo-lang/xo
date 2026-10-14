# Thread local words collide across parallel branches

Status: Observed 2026-10-14 (merging work item B02 after the B05 unwinder).

## Symptom

Two branches each claimed word 7 of the native thread local table
(rt.tlocal_get, sched.xo TLS_*): the B05 unwinder (unwind.xo UNW_TLS)
and B02's std/log attribution (logstate.xo TLS_FROM, with TLS_FROM_N in
word 8). The code merged cleanly; only the comment in sched.xo that lists
the words conflicted. Had the comment not conflicted, a log call between
an unwinder store and load would have overwritten the unwinder's scratch
record, with no build error.

## Cause

The word numbers are plain constants in each module; nothing checks that
two modules use different words of the 16 word table.

## Fix

The merge kept the unwinder on word 7 and moved logstate.xo to words 8
and 9. When a module takes a thread local word, update the list in
sched.xo's TLS_* comment in the same commit, and on a merge grep
`tlocal_get(`/`tlocal_set(` constants across std/native/llrt for
duplicates.
