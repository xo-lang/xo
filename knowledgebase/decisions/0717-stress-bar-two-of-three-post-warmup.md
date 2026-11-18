# 0717: The stress memory bar marks after warm-up and passes on 2 of 3 runs

Status: Accepted (owner, 2026-11-18; proposed by the lead from xo-4c's #310 findings). Amends 0616's stress kind.

## Context

0616 (as amended 2026-11-17, xw answer 68) passes a stress run when the
median resident memory of its last 3 minutes is under 10% above the
median of minutes 5 to 7, and one run decides.

Two findings from xo-4c on 2026-11-18 (task #310, notes/xo-4c/done-310.md):

- The verdict is noisy run to run on the same commit and host: otlp's
  run before #310 read +65%, +5% and -1% in three runs. sse, grpcreflect
  and debug each passed one run and failed the next.
- Minutes 5 to 7 sit on the warm-up rise for some programs. sse alone at
  main: RSS rises to 84 MB by minute 9, falls to 73 MB by minute 13 and
  stays flat to minute 30. The mark measured warm-up, not retention.

## Decision

1. The mark is the median of the samples of minutes 15 to 17 (was 5 to
   7), against the median of the last 3 minutes' samples. The bar stays
   under 10%. The run stays 30 minutes, so the retention window is the
   last 13 minutes after the mark.
2. A module's stress kind passes when 2 of 3 runs on the same commit
   pass the memory bar. Every run is recorded in `stress.md`, failures
   included. A mismatch or fault in any run fails the kind (the 2 of 3
   rule is for the memory bar only). A run that passes first and second
   needs no third.
3. The `--check=all` leakcheck run and the server load shape are
   unchanged.

## Consequences

- internal/stdtest TestStressRssMedians and the stdproven stress verdict
  (and stdcov's reading of `stress.md`) change to the new mark and the 2
  of 3 rule (task for xw-1).
- Real retention is still caught: a leak grows through the last 13
  minutes, and grpcreflect's interleaved small-class pages (#310) keep
  failing until the allocator fix.
- Evidence already recorded under the old mark stays valid where it
  passed.

## Alternatives

- Keep one run and the 5 to 7 minute mark: rejects modules on noise and
  warm-up (otlp, sse).
- A longer run (45 minutes) with the mark at 15: costs host time on every
  module for the same signal.
- Median of 3 runs: needs all three runs every time; 2 of 3 stops after
  two passes.
