# 0452: A List grows by Go's slice curve, not by doubling

Status: Accepted (owner, 2026-10-31: "1 and 2 are yes", Go's whole growth curve; proposed by xo-lang-2)

## Context

A native List doubles its capacity when a push finds it full
(std/native/llrt/obj.xo grow): a list that just grew holds half its
capacity unused, and a list that grows to its final length and stays
holds a quarter of it unused on average (between 0 and half). Decision
0448's work on xo's self build found the doubled tables everywhere in
emit's peak: the SSA value tables (insert_rc and the inliner push into a
full table), the register allocator's sets and ranges, the instruction
arrays. Packing them one by one costs a copy each (ssa.pack_values: max
RSS -4% but wall +0.9% on linux/arm64), and there is no in-place shrink.

Go's growslice doubles a slice up to 256 elements, then grows it by a
quarter plus 192 elements (newcap += (newcap + 3*256) >> 2), a smooth
step from 2x toward 1.25x: a large slice holds at most about a fifth of
its capacity unused.

## Decision

grow uses Go's curve: under 4 elements, 4; under 256, double; from 256,
cap + (cap + 768) / 4. Lists under 256 elements grow as before. Nothing
else changes: the element layout, with_capacity, the immortal empty
list's first push (4).

## Consequences

- xo's self build on the Mac (two rounds, against 8e7a90ea6): emit peak
  6009, 6047 -> 5357, 5308 MB; max RSS 5884, 5926 -> 5644, 5593 MB; peak
  footprint 5.43, 5.42 -> 5.24, 5.18 GB; wall 47, 47 -> 46, 46 s.
- linux/arm64 `xo bench --ab` (the self build, ten rounds, 8e7a90ea6 against the change): wall 0.963 (0.959 to 0.968), CPU 0.986 (0.985 to 0.988) and max RSS 0.935 (0.914 to 0.943), all better.
- Push-bound programs copy more often past 256 elements (about 3x as many
  resizes for a list growing to 20 million): bench/perf/alloc grow and
  mixed on linux/amd64 (ten rounds); 20 million pushes into
  one list on the Mac (medians of five, the program's own clock): Xo
  before 29.6 ms and 301 MB max RSS, Xo after 29.6 ms and 212 MB, Go
  57.7 ms and 340 MB; alloc grow 3.63 -> 3.38 ms (Go 15.7 ms).
  linux/amd64 `xo bench --ab` (ten rounds, about a second a run: alloc
  grow 300 and mixed 40, the repeat count added for it): grow wall 0.892
  (0.885 to 0.904) and mixed 0.984 (0.978 to 0.991), both better. (A
  single 3 ms run read 1.228 worse: process start, not the pushes.)
- A list past the mid classes (over 16 KiB) resizes through
  rt.heap_big_resize (alloc.resize), not the allocate-copy-free of hmove.
- Output is unchanged (a List's capacity is not observable).
- The wasm runtime (compiler/wasm/rt.xo) keeps doubling (to be ported if
  its peak matters); the Go backend already uses Go's slices.

## Alternatives

- Pack after the passes (ssa.pack_values, serial or on the emit's tasks):
  -4% max RSS for +0.9% wall; packing on the emit worker made the peak
  worse (the waiting parts still hold the doubled tables).
- A runtime List.shrink (madvise the unused tail of a large block): no
  copy, but a new prelude method on every backend and a call at each
  site that knows a list is final.
- Go's step only past the size classes (lists in the classes keep
  doubling, since the mid classes double): the Mac emit peak only about
  -4.5% (6064, 5999 -> 5752, 5784 MB) and a larger footprint; alloc grow
  0.858 better but mixed's CPU 1.005 worse (linux/amd64, ten rounds).
  Go's whole curve wins on every measure.
- A fixed 1.5x factor: fewer resizes than 1.25x and more waste; Go's
  curve is measured and documented, and std follows Go.

## Addendum 2026-11-01: capacity rounds up to the block

Owner, 2026-11-01, accepted through the lead. As Go's `roundupsize`, a growing List
takes what its block holds (`block_size` in std/native/llrt/obj.xo): the classes above
1 KiB are powers of two, so a capacity of 128 or 256 elements plus the header took the
next class half empty. linux/arm64 self `--ab` (9c49bc74f vs 3bb078e20, 3 rounds, 0
loaded, every effect under 3%): wall 0.996 (0.995 to 0.998), CPU 0.998 (0.997 to 0.999),
max RSS 0.986 (0.986 to 0.992). An earlier run on cf8fdebfa read wall 0.998, CPU 1.001,
max RSS 0.98. `alloc grow` wall 0.922 (0.922 to 0.927), `alloc small` 1.0, both with max
RSS 1.0. The self build's gate: bootstrap s1, s2, s3 and fixed point ok.
