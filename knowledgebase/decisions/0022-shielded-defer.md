# 0022: defer blocks are shielded from cancellation

Status: Accepted (2026-10-07)

## Context
Sticky cancellation (0015) made cleanup in `defer` fail with `Canceled`, so
temp files leaked after a timeout. (Paper test 2026-10-07 spec 0.2, [results](../../bench/results/paper-2026-10-07-v0.2.md))

## Decision
Inside deferred code, cancellation points do not report cancellation. A
shielded `defer` running longer than 5 seconds faults (XO-F005).

## Alternatives
- An explicit `task.shield { }`: easy to forget exactly where it matters.
