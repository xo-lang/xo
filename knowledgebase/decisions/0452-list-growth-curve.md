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

## Addendum 2026-11-01: a list of big elements starts at one slot

Owner, 2026-11-01, through the lead. A new list had room for at least 4 elements
(`obj.rl_new_n`, the first push into an empty literal's list, `rl_copy`, and `grow`'s
floor), so `tlssrv.save`'s list of one `Conn` (1,872 bytes: a `Server` of 1,792 bytes
holding a 248 byte `Conf` copy, a `Hello`, two `Keys` and a certificate) took the 8,184
byte class, 64% of a TLS server's in-use bytes (XO_PROF=heap at rate 512, 300 held
connections; 2,000 held connections: RssAnon 111 to 115 MB against 79 to 81 MB for Go's
crypto/tls). The floor is now `obj.min_cap(esize)`: 4, or 1 when an element is over 256
bytes (four would be over 1 KiB). `with_capacity(n)` takes the larger of n and the floor,
as before. The same rule stands in compiler/llvm/call.xo (`list.new`, the header it
writes) and compiler/wasm (`$min_cap`, `$rl_new_n`, `$rl_grow`, the empty literal); arm64
calls obj.xo. Lists of elements up to 256 bytes are unchanged, so their output is too.
Growth from the floor of 1 doubles (1, 2, 4, then 4 elements as before), so a list of
big structs that grows to many elements costs up to two extra copies.

Growth from the floor: the first growth of any list goes to 4 slots, so a big-element list that grows from 1 pays one extra copy, not the 1, 2, 4 steps (measured: alloc bign 1.165x with plain doubling from 1).

Addendum (2026-11-03, Xo Lang 2): the curve's cost on darwin. Lang 4's 0.36.0 comparison found
coll push 1.152x of v0.35.0 on darwin (0.952 on linux/arm64) and bisected it to this decision.
heapos.xo large_resize moved a growing large block into a mapping with room for twice its new
size, so its next growth stayed in place; that room fit doubling (a copy every 4x of growth) but
not this curve (a copy every 2.4x), and each copy faults in fresh pages. The room is now four
times the size (a copy every 4.9x); the reserved pages are not touched and the memory budget
counts the bytes handed out, not the room. Darwin, xo bench --ab of main's coll against the
change: push wall 0.863, instructions 0.863, max RSS 0.741; against the doubling curve: push
wall 0.901, max RSS 0.815. Linux/arm64 ab-host (3 rounds): push wall 1.010 (within noise), max RSS 0.763; putget wall 1.011 (within noise), max RSS 0.880; the self build wall 1.006 (a 3-round "worse" under 1%, 2 of 3 rounds loaded), max RSS 1.001.

The reservation is not memory: XO_MAX_MEM counts the bytes handed out, so one list grown to 320 MB
under XO_MAX_MEM=512M (about 1.3 GB reserved) runs on darwin and linux/arm64, and one grown to 1 GB
stops with the budget's error (617 and 514 MB mapped); golden mem_budget_growth holds a 24 MB list
under 64 MB. Linux's default overcommit (heuristic, mode 0) and mode 1 allow the reservation; in
strict mode 2 it counts toward the commit limit (not measured: the hosts run mode 1).
