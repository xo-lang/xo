# 0726: perf evidence reads interleaved rounds as the median of per-round ratios

Status: Proposed (xo-4c, 2026-11-20, task #429; for the owner)

## Context

Decision 0461 makes a std module Proven on performance when every bench
twin row runs at or under 1.05 of Go; `stdproven perf` runs
`scripts/twincheck -time 9` (Xo, then Go, nine times) and reports the
ratio of the two medians. On linux/arm64 host 2, the perf evidence host,
a short CPU loop is bimodal from run to run (about 5 or about 8 ms for
the gobridge `none` loop), for Go's own twin as much as for Xo, in an
exclusive lease, pinned or not (knowledgebase/infrastructure/offload.md,
"Timing on host 2"). Nine rounds per side can land either program in
either mode, so one row's ratio moves by more than the 5% bar between
runs: std/go's `none` row read 1.483 at aa7b2c4c3, 1.04 by median over
20 interleaved rounds, and 1.15 over 300.

## Decision (proposed)

Evidence to follow, in this file: per-round Xo and Go times of every
std/go and brotli row on host 2, with what each statistic gives.

## Consequences

To fill in.

## Alternatives

To fill in.
