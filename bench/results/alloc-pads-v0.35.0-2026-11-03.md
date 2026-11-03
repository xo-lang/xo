# /alloc across code placements: v0.35.0 against itself (2026-11-03)

Question: the 2026-11-03 bisect of a /alloc slowdown since v0.35.0 found
two steps (c92c99eff, 4be6231f5), each about 2 percent at one build per
side. Each of those revisions measured 0.958 to 1.003 of v0.35.0 when
built at four code placements (`XO_ARM64_PAD` 0 to 3, nops before every
function that shift the whole program). Is v0.35.0's own number a lucky
placement?

Answer: yes. v0.35.0's own source at pad 0 matches the release server
(1.003 and 1.000 at 32 connections), and at pads 1 to 3 runs 0.974 to
0.978; main (f3f9552ac) at 0.975 sits where v0.35.0 sits at three of its
four placements. The /alloc row was placement, not a regression, and
neither bisect step is one. Since 7e1b953fe, alloc-split.sh and the
release report's http section build each side at four pads and judge by
the median over pads (bench/perf/jitter/pads.py).

## Method

- Job `lang1v35` (Xo Lang 1, 2026-11-03):
  `bench/perf/jitter/alloc-split.sh last "v0.35.0+XO_ARM64_PAD=0,v0.35.0+XO_ARM64_PAD=1,v0.35.0+XO_ARM64_PAD=2,v0.35.0+XO_ARM64_PAD=3,v0.35.0+XO_ARM64_PAD=0,f3f9552ac" lang1v35 3`.
- Builds on the Mac (alloc-build-mac.sh, 53 s), linux/arm64 cross
  builds: each spec's xo built from its revision's tree (216203685, the
  v0.35.0 tag, or main f3f9552ac) with the darwin v0.35.0 release, each
  compiling the jitter server source of the last spec (MODE `last`, main's
  bench/perf/jitter/xo) with its own tree's std. `base` is the released
  v0.35.0 xo building the same source. p4 repeats p0 as a control.
- Measured on the first linux/arm64 offload host (12 CPUs) in one
  exclusive lease (3,511 s): server on CPUs 0 to 5, loadgen on 6 to 11,
  `-d 10s -warmup 2s -awake`, closed loop /alloc at 32 and 256
  connections, 3 rounds, servers interleaved in a rotating order. Load
  before the rounds 0.95 after a 55 s quiet wait.
- Log: the lead's notes, `~/xo-lead/notes/lang1/alloc/v35.log`.

## Results

Median req/s over 3 rounds, the rounds' range in brackets, the ratio to
`base`, and the server's CPU microseconds per request.

| server | build | c32 req/s | c32 vs base | c32 CPU us/req | c256 req/s | c256 vs base | c256 CPU us/req |
|---|---|---|---|---|---|---|---|
| base | v0.35.0 release xo | 164,337 [164,227 to 164,499] | 1.000 | 33.74 | 182,034 [181,960 to 183,516] | 1.000 | 32.71 |
| p0 | 216203685, pad 0 | 164,807 [163,943 to 165,075] | 1.003 | 33.72 | 184,013 [182,973 to 184,014] | 1.011 | 32.38 |
| p4 | 216203685, pad 0 (control) | 164,266 [164,241 to 164,469] | 1.000 | 33.76 | 182,820 [182,218 to 182,928] | 1.004 | 32.59 |
| p1 | 216203685, pad 1 | 160,734 [160,484 to 161,135] | 0.978 | 34.70 | 178,297 [177,820 to 178,472] | 0.979 | 33.42 |
| p2 | 216203685, pad 2 | 160,014 [159,331 to 160,197] | 0.974 | 35.06 | 175,755 [165,461 to 176,969] | 0.966 | 33.91 |
| p3 | 216203685, pad 3 | 160,510 [160,104 to 161,269] | 0.977 | 34.76 | 177,938 [177,370 to 178,151] | 0.977 | 33.46 |
| p5 | f3f9552ac (main), pad 0 | 160,307 [160,039 to 160,548] | 0.975 | 34.81 | 176,812 [176,733 to 176,902] | 0.971 | 33.67 |

The pad 0 control agrees with pad 0 (1.000 and 1.003 at 32 connections),
so the 2.2 to 2.9 percent between pad 0 and pads 1 to 3 is placement,
not run noise. Aligning every function to 64 or 128 bytes did not remove
that spread (gotchas/arm64-loop-alignment-stable-not-fast.md, 2026-11-03).
