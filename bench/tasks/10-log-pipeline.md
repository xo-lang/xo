# Task 10: Log ingestion pipeline

Build a program that ingests log lines and keeps per key metrics, without
losing lines on shutdown.

## Requirements

- Input: lines on standard input in the form `<unix_millis> <level> <service> <message>`.
  Malformed lines are counted and skipped, never crash the program.
- A bounded queue (capacity 1000) between the reader and N parser workers
  (`--workers`, default 4). The reader blocks when the queue is full
  (backpressure); it never drops lines.
- Workers aggregate per service: count by level, first and last timestamp,
  and the 5 most frequent messages.
- Every `--flush` interval (default 10s) the current aggregates are written as
  one JSON object per service to `--out` (a directory), one file per flush,
  written atomically (temp file then rename).
- On end of input or on SIGINT/SIGTERM: stop reading, let the workers finish
  everything already queued, write a final flush, and exit 0. No accepted line
  may be lost.
- Print a summary at exit: lines read, parsed, malformed, flushes written.

## Tests to write

- Every valid line is counted exactly once with several workers.
- Malformed lines are counted and skipped.
- The reader blocks rather than dropping when workers are slow.
- Flushes happen on the interval (fake clock) and files are written atomically.
- Shutdown in the middle of input writes a final flush that includes every
  line read before the signal.
