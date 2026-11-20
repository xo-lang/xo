# 0726: perf evidence reads interleaved rounds as the median of per-round ratios

Status: Accepted (owner, 2026-11-20, xw decision #201: host 2 perf evidence is the median of paired per-round Go/Xo ratios over at least 30 interleaved rounds, for every std module; proposed by xo-4c, task #429)

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

## Evidence (host 2, 2026-11-20, one exclusive lease, load 0.6)

Every std/go (bench/perf/gobridge) and brotli twin row, 60 interleaved
rounds (Xo, then Go), the binaries cross-built outside twincheck
with an xo from before 04eaa39a1 (#467) (so the levels are not the modules' evidence, only how the
statistics compare on the same times). Brotli's levels here (enc1 0.50, enc11
1.34) differ from std/evidence/brotli/perf.md's twincheck builds; read
only the spread between columns. The last column is the ratio of
medians, today's statistic, over six disjoint 9-round windows of the
same 60 rounds: what one `stdproven perf -rounds 9` run could print.

| row | Go ms | Xo ms | ratio of medians | median of ratios | trimmed mean of ratios (20%) | ratio of mins | 9-round windows |
|---|---|---|---|---|---|---|---|
| gobridge/none | 6.0 | 8.3 | 1.397 | 1.377 | 1.353 | 1.196 | 1.051 to 1.592 |
| gobridge/str | 26.1 | 27.2 | 1.041 | 1.041 | 1.041 | 1.043 | 1.036 to 1.046 |
| gobridge/bytes | 18.7 | 25.2 | 1.347 | 1.374 | 1.368 | 1.478 | 1.314 to 1.401 |
| gobridge/int | 108.5 | 105.1 | 0.969 | 0.967 | 0.963 | 0.972 | 0.958 to 0.971 |
| brotli/none | 0.8 | 0.7 | 0.832 | 0.827 | 0.833 | 0.824 | 0.801 to 0.865 |
| brotli/text | 50.5 | 48.7 | 0.964 | 0.965 | 0.964 | 0.976 | 0.950 to 0.977 |
| brotli/html | 24.1 | 23.7 | 0.984 | 0.962 | 0.963 | 1.004 | 0.911 to 1.023 |
| brotli/mixed | 24.5 | 21.4 | 0.873 | 0.883 | 0.879 | 0.899 | 0.856 to 0.887 |
| brotli/enc1 | 35.5 | 17.9 | 0.503 | 0.513 | 0.509 | 0.548 | 0.456 to 0.534 |
| brotli/enc6 | 22.8 | 26.6 | 1.165 | 1.168 | 1.167 | 1.299 | 1.103 to 1.232 |
| brotli/enc11 | 174.6 | 233.1 | 1.335 | 1.331 | 1.335 | 1.325 | 1.323 to 1.355 |

Two things show:

1. The statistic barely matters at 60 rounds: on every row the ratio of
   medians, the median of per-round ratios and the trimmed mean agree to
   0.03 (brotli/html 0.984 against 0.962 is the widest). The ratio of
   mins leaves them on rows where one side has a rare fast run
   (gobridge/bytes 1.478, brotli/enc6 1.299), which is why min is not
   proposed: it rewards a lucky run.
2. The round count matters: nine rounds of the short gobridge/none row
   (6 to 8 ms, the bimodal range) read anywhere from 1.051 to 1.592;
   rows of 18 ms and longer stay within 0.12 across windows.

The 300-round run on the none loop (s429, 10M iterations) agrees: ratio
of medians 1.145, median of ratios 1.157, trimmed mean 1.144, against
1.04 and 1.48 from 20 and 9 round runs.

With #467 (04eaa39a1, an xo built at 486698b71), 60 more rounds of the
gobridge rows in a second lease (median of per-round ratios): none 1.208,
str 1.123, bytes 1.133, int 0.941. bytes moved as #467 predicted (25.2
to 21.6 ms); str moved the other way (27.2 to 29.1 ms) with no change on
its path, which a statistic cannot fix: it is a code or layout effect,
looked at under task #429.

Raw rounds: ~/xo-lead/notes/xo-4c/s726/raw-h2.out, raw2-h2.out (outside
the repository); the job scripts beside them.

## Decision (proposed)

`stdproven perf` keeps twincheck's alternation and judges a row by the
median of its per-round Xo / Go ratios (each round pairs one Xo run with
the Go run after it, so a slow VM phase hits both sides of a pair), with
at least 30 rounds, and more for a row whose Go side is under 10 ms
until each side has run 1 s in all. The table keeps the Go and Xo
medians in ms and adds the per-round ratio column the verdict reads.
The trimmed mean (20%) is the alternative if the owner prefers a mean;
on this evidence it gives the same verdicts.

## Consequences

- Rows move by up to 0.03 from the statistic; short rows stop swinging
  by 0.5 between runs.
- A 9-round run costs about 3 times as many runs (30 rounds); every
  module's perf.md is refreshed on its next run under the new reading,
  no rewrite of old files.
- Until the owner decides, current perf.md results stand.

## Alternatives

- Keep the ratio of 9-round medians: the gobridge/none windows span
  1.05 to 1.59 on one host in one lease.
- Ratio of mins: rewards a lucky run (gobridge/bytes 1.478 against 1.37
  for the others).
- Pin a core or set GOMAXPROCS=1: the slow mode stays (s429 ctl run).
- Move perf evidence off host 2: the arm64 evidence runs there
  (SP_ARCH_PIN=1 with offload-arm64b.env); this keeps the host and reads
  its times robustly instead.
