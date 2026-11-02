# 0453: xo ships its own layout file

Status: Accepted (owner, 2026-11-01). Proposed by xo-lang-2, 2026-11-01. Extends 0403; changes its item 5 for one case
(a file with no target line). Serves 0448 case (c).

## Context

A small build touches about 18 MB of xo's 34 MB of code on the Mac (27 MB of file pages on
Linux) because the functions it runs are spread through the binary: about a quarter of coll's
RSS. Decision 0403 stage 1 orders functions by a sampled profile. Sampling is the wrong input
here: 80 profiles of warm and edit builds at 10 kHz (89,311 samples) named 1,885 functions and
moved coll only from 85 to 83 MB. Most of the touched code runs once or a few times and no
sampler sees it. Callgrind sees every function: 2,913 over a warm and an edit build of coll and
conc-spawn, 11.9 MB of code.

## Decision

- `compiler/xo.layout` is checked in. A build of compiler/xo reads it as any main package's
  xo.layout (0403 item 5), so the self build, seeds and releases all use it.
- It has no `target` line, which every xo already reads as any target (an xo older than this
  file would not know a new `target any`, and the first stage of a build is made by one). 0403
  ignores a foreign target's profile because a timing profile says little about another target's
  placement; this file says which functions run, and their names are the same on every target.
- `scripts/xolayout.sh WORK` remakes it: callgrind runs of those four builds in the linux/arm64
  Docker image, reduced by `scripts/xolayout` (Go). A stale file costs only placement (0403: a
  function that disappeared is ignored, a new one goes with the cold code), so it is remade by
  hand when the rows drift, not on every change.
- The internal/xotool source key includes xo.layout, so a change to it rebuilds xo.

## Consequences

Mac, 3 rounds, no stats, RSS / footprint MB, against the commit before: coll warm 64-66 / 41-42 to
58-59 / 39, edit 64-66 / 41 to 59 / 39-40; conc-spawn warm 55-56 / 34-35 to 53-54 / 35, edit 56-57 /
34 to 52-54 / 34-35. __TEXT resident, xo's and the system libraries', 23.6 to 18.6 MB on coll. That meets
0448 case (c)'s 60 MB for both programs. Wall on the Mac was not comparable: load about 75.

linux/arm64 --ab, 3 rounds, 0 loaded: coll's edit wall 1.003 (0.993 to 1.006), max RSS 0.915 (0.914
to 0.918), 128 to 117 MB; conc-spawn's edit wall 0.908 (0.887 to 1.135, spreads 16 and 25%, no
claim), max RSS 0.926 (0.831 to 0.926), 109 to 101 MB. An A/A control on coll's edit read wall 0.992 and
max RSS 0.969 on this host, so a coll RSS move under about 3% is no claim. Self build: wall 1.006
(1.002 to 1.006), CPU 1.005, max RSS 0.997: the tool says worse, inside the host's 1% band.

- A new input in the fixed point set: compiler/xo.layout. Every stage reads the same file, so
  s2 and s3 stay identical.
- The file is made from small builds only. Functions only a large build runs keep their declared
  order after it. The self build's 0.6% is inside the noise band; if it holds up, the next step
  is adding the self build's hottest functions after the small builds' list, which keeps the small
  builds' pages together.

## Alternatives

- Sampling more (rejected above: 1,885 of 2,913 functions, a fifth of the gain).
- An instrumented xo that records functions on entry: exact, and no valgrind, but a second build
  kind to maintain.
- A list per target: more files for names that are the same.
- A hot and cold split inside functions (0403 stage 2): held by 0403's measurements.
