# 0579: Loop head alignment on x86

Status: Proposed (Xo Lang 4, 2026-11-08; the lead asked for it after
0577's Intel result; measurement first).

## Context

Xo's x86 emitter aligns functions to 4 bytes and never aligns a loop
head, so a loop's bytes sit wherever the code before it ends. The
assembler keeps jumps off 32 byte boundaries (decision 0398's JCC
padding) but places nothing else. On an i7-9750H (Coffee Lake) the same
four uop byte loop ran 316 ms in one placement and 508 ms in another
(bench/perf/bytesum, 63 bytes x 20M calls; decision 0577's Results).
An opt-in experiment (XO_PASSES=+x86_loop_align16, +x86_loop_align32:
a `.p2align` before every block a later emitted block jumps back to),
7 rounds, median ms:

| bytesum | no alignment | 16 | 32 |
|---|---|---|---|
| 0577 off | 408.8 | 405.7 | 404.9 |
| 0577 on | 511.5 | 398.6 | 328.5 |

## Decision

To be decided by measurement:

- variants: every loop head to 32; to 16; innermost loops only; a cap on
  the padding (as LLVM's align-loops with a max skip);
- --ab on Zen 5 (the amd64 host) and the i7-9750H over bench/perf;
- __text size of xo on amd64 for each variant;
- the amd64 sample with the chosen variant on;
- a follow-up row for arm64 (Apple M on the Mac, Graviton on host 1),
  where the same placement effect may exist.

A default flip is a default change: its gate runs the touched packages'
full suites.

## Consequences

To be filled from the measurements.

## Alternatives

- Align functions to 16 or 32 only (cheaper, but a loop's offset inside
  its function still varies).
- Leave placement alone (today; Intel loops vary by up to 60%).
