# 0007: Structured concurrency replaces bare goroutines and ctx

Status: Accepted (2026-10-07). Amended by 0032.

## Decision
Tasks are spawned only from a `scope` handle and never outlive it. Failure
cancels siblings. Deadlines, cancellation, request ids, and trace spans flow
through the scope tree implicitly. Channels and `select` are kept.

## Consequences
- Goroutine leaks are impossible by construction.
- No `ctx context.Context` parameters.
- The runtime always has a task tree for `xo dump` and deadlock reports.

## Evidence
- Harder tasks 2026-10-07: fire and forget refresh in a cache needs a daemon and channel (07); planted leaks become hangs, not compile errors. ([results](../../bench/results/planted-and-harder-2026-10-07.md))
- Planted bugs in Xo 2026-10-07: leak variants became hangs or slow shutdowns; with `signal_after`, a foreground daemon still shuts down cleanly on SIGTERM. ([results](../../bench/results/planted-xo-2026-10-07.md))
