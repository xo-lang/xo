# Notes: 10-log-pipeline

## Ambiguities resolved

- Line format: split on single spaces into exactly 4 parts. Timestamp must be a non negative base 10 integer; level, service and message must be non empty (message may contain spaces; whitespace only message is malformed). Level is normalized to upper case and any level token is accepted. Empty lines count as read and malformed. A final line without a trailing newline is processed; `\r\n` is tolerated.
- Parsing happens in the workers (the task calls them parser workers), so the reader only counts lines read and the workers count parsed and malformed.
- "Read" and "accepted" are the same point: a line is counted as read when the reader loop takes it from the stdin goroutine, and from then on it is always sent (blocking) to the queue, even if a signal arrives while the reader is blocked on a full queue. Workers keep draining after the signal, so that send always completes and no accepted line is lost. A line that the stdin goroutine has pulled from the OS but not yet handed over when the signal arrives is not accepted (and not counted).
- Stopping on a signal does not wait for stdin to return; the stdin reading goroutine is abandoned (it exits on EOF or when the process exits).
- Aggregates are cumulative for the whole run (each flush is a full snapshot, not a delta). First and last timestamps are the min and max seen, since workers process lines out of order.
- Top 5 messages: sorted by count desc, ties broken by message text asc. All distinct messages are kept in memory per service (unbounded); a bounded sketch was not requested.
- Flush file format: JSON Lines, one object per service sorted by service name, file `flush-NNNNNN.jsonl` in `--out` (created if missing). A flush with no data still writes an (empty) file and counts as a flush. Atomic write: temp file in the same directory, fsync, close, rename; temp removed on failure.
- Interval flushes are driven by a tick channel; production uses `time.Ticker(--flush)`, tests inject a fake tick channel. The interval flusher is stopped before the final flush so flush numbering is deterministic.
- Summary is printed to stderr: `lines read: N, parsed: N, malformed: N, flushes written: N`. Exit 0 on normal end or signal; 1 if a flush or stdin read failed; 2 on bad flags.
- `--flush` takes a Go duration (`10s`, `500ms`).
- Queue capacity is fixed at 1000 in the CLI; `Config.QueueSize` exists only so tests can use a small queue (one test also checks the default of 1000).
- Test observation uses unexported hooks (onRead, onQueueFull, beforeParse, afterParse, onFlush) instead of sleeps.

## Bugs caught by the loop

- Run 1: a wrong expectation in my own test (which service owns the last timestamp in the generated input). The production code was correct; no code bugs were caught.
