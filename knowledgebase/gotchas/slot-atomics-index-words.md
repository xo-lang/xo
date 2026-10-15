# rt.slot_* index words, not bytes

Status: Observed (2026-10-15, B22 remnants batch 2, found by xo-2b's
regression sweep)

Symptom: from 0e1771fd on, `xo test --no-gc --backend=arm64` hung for
good on tests that sleep under the test world's virtual clock
(internal/gen golden test_shutdown, test_signal_after,
examples/notes-service; cmd/xo TestNoGCTests and TestNoGCTestLeaks):
the sleeping task parked and woke in a loop while twrun.xo kick moved
the clock and fired timers.

Cause: poll.xo now(), the timers' clock, read the virtual clock with
`rt.slot_load(tw, TW_VNOW, ACQUIRE)`, where TW_VNOW = 8 is the byte
offset of tw.vnow (xo_rt.c, asserted). A slot atomic's index counts
words (the cell is xs + 8 * i, std/native/rt.xo), so it read word 8,
byte 64, past the record. Until batch 2 nothing in the test world used
now(): clock.sleep's deadline was the C's (tnow). Batch 2 moved the
sleep to poll.xo clock_wait, whose deadline from now() was then
garbage, a time the virtual clock never reached.

Fix: `rt.slot_load(tw + TW_VNOW, 0, ACQUIRE)`. With a constant that is a
byte offset, address the cell as base plus offset and index 0 (as
twrun.xo and poll.xo's ev_start do); keep word indexes (pool.xo's DLK_*,
P_NW) only where the constant says it is a word index. -short runs none
of the no-GC test runs: a change to the test world's clock or sleeps
needs cmd/xo TestNoGCTests through heavy.sh.
