# A polling busy retry starves a writer against writers that come straight back

Status: Observed 2026-11-18 (task #229, Xo Lang 3)

## Symptom

A program with 8 tasks inserting into one SQLite WAL file in a loop, each on its own
connection, and a 9th task making one insert, saw the 9th insert fail with
`SQLITE_BUSY: database is locked` after its full `_busy_timeout` (2 s) in 9 of 10 runs on
the Mac. The hot writers sometimes failed the same way. Go's mattn/go-sqlite3 failed the
same program in 4 of 20 runs.

## Cause

SQLite's busy handling is a poll with no queue: a connection that gets SQLITE_BUSY naps
(std/sqlite: 1 ms growing to 16 ms, parked in Xo since task #94) and tries again. A
writer that has just finished a statement steps again at once and takes the lock before
any napping writer wakes, so the one that napped loses nearly every release until its
deadline. Retrying more often only narrows the window; it does not give a turn.

## Fix

std/native/sqlite/xo_sqlite.c keeps a write gate per database file in the process (a
FIFO of writers that got SQLITE_BUSY, under SQLite's static application mutex). Once the
oldest waiter has waited 50 ms, counted also from the last such turn, only it may step;
other writes outside a transaction wait. A strict line for every write cut throughput
about 30 times (every hot writer queued); restarting the 50 ms after each served turn
kept main's throughput and still served the cold writer in 0 to 608 ms.

Limits: only one process is covered (other processes still poll), and statements inside
a transaction or reads are not gated (their locks may be what the head waits for).
Test: std/sqlite_test.xo "a writer beside busy writers gets its turn before its busy
timeout".
