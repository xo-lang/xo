# Notes: 10-log-pipeline (strict Go)

## Task ambiguities and decisions
- Line format: fields separated by single spaces; message is the rest of the line after the third space and must not be blank. Double spaces between the first three fields, a negative or non integer timestamp, missing fields, and blank lines are malformed. Trailing `\r` is stripped. Level is upper cased so `info` and `INFO` aggregate together; any non empty level token is accepted (no fixed level list).
- Lines longer than 64 KiB are skipped and counted as read and malformed (they never reach the queue), so an oversized line cannot crash or stall the program.
- "lines read" counts lines accepted from the input (queued, or counted malformed in the reader for too long lines). A line still inside the bufio buffer or the input goroutine when the signal arrives was not accepted and is not counted; every accepted line is aggregated before the final flush.
- Stopping on a signal: the blocking stdin read runs in its own goroutine so SIGINT/SIGTERM is honored even when stdin is idle. That goroutine may remain blocked in Read at exit; the process exits anyway.
- Aggregates are cumulative across flushes (each flush is a full snapshot, not a delta). First and last timestamp are min and max of observed timestamps, since workers process lines out of order.
- Flush file format: `flush-NNNNNN.json`, one JSON object per service per line (JSON Lines), sorted by service, written to a `.flush-*.tmp` file in the same directory, fsynced, closed, then renamed. A flush with no data still writes an (empty) file. Each object includes flush_seq and flushed_at.
- Top 5 messages: ties broken by message text ascending. Exact counts are kept for every distinct message (unbounded memory for high cardinality messages; a sketch would be needed in production).
- Summary is printed to stdout; diagnostics to stderr. Exit 0 on clean end or signal, 1 if a read or flush failed, 2 on bad flags. `--out` is required (no default given).
- Fake clock: the flush ticker is injected as a channel (`Config.Ticks`) and the flush timestamp as `Config.Now`; main wires a real `time.Ticker`. Tests use hooks (ReaderBlocked, Accepted, BeforeProcess, AfterProcess, Flushed) to synchronize instead of sleeping.

## Bugs caught by each tool
- errcheck: 5 unchecked writes to stderr in main.go (real, though low impact).
- Tests: 1 test-failure, a bug in the test expectation (wrong service for the last timestamp), not in the implementation.
- vet, staticcheck, nilaway, exhaustive, race: nothing found.

## False positives and suppressions
- No tool false positives.
- One deliberate `_, _ =` in `warn()` (main.go): the error from writing a diagnostic to stderr is ignored because there is nowhere left to report it and the exit code already signals failure. No nolint comments.
- Test helper `ParseLine` result in TestParseLine uses `e, _ :=` to discard a bool (not an error).
