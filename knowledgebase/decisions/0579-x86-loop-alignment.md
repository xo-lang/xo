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

## Results (2026-11-08)

Variants (opt-in passes on branch l4-loopalign, not pushed): 32 (every
loop head: a block a later emitted block jumps back to), 32i (innermost
heads only), 32c (32 with at most 10 bytes of padding, `.p2align
5,,10`, a form the assembler gains), 16.

xo's own __text (darwin/amd64, 121,524,744 bytes with none): 32 +5.6%,
16 +2.4%, 32c +0.54%, 32i +0.25%.

Default code (decision 0577 off), 10 bench/perf programs:

- Zen 5 (amd64 host, --ab, 3 rounds; sort did not run, its path is
  sort/xo): within noise for most; better: mapops 0.954 (32) and 0.93
  (32c), strconv 0.997 and bytesum 0.996 (32), sha3 0.964 (32c); worse:
  bytesum 1.001 (32i), jsontoken 1.065 (32c, a 1 ms program).
- i7-9750H (5 rounds, the first dropped for macOS's first run check):
  every variant within noise of none on all ten (bytesum about 405 ms,
  mapops 412, sort 76, adler32 42).

Decision 0577's loop (bytesum with acc_range), i7-9750H, 7 rounds,
median ms: off 404.1, on 510.6; on with 32: 326.6; 32i: 398.1; 32c:
510.7 (the head needed more than 10 bytes, so the cap left it as is).
Only aligning every loop head gives the 36% gain; aligning only the
innermost head recovers the loss but not more (the outer loop's
placement matters too).

## Consequences

Today's code neither gains nor loses much from alignment on either
core; the large Intel effect shows when a change shortens a hot loop
(0577), where the placement decides between a 27% loss and a 19% gain
over today. Open: whether 32's 5.6% __text is worth that for loops that
change shape, or 32 only together with 0577 (the owner decides; the
lead recommends after the arm64 row).

## Alternatives

- Align functions to 16 or 32 only (cheaper, but a loop's offset inside
  its function still varies).
- Leave placement alone (today; Intel loops vary by up to 60%).
