# Str sort in place: both stable in-place sorts measured, neither keeps the
# instruction count, 2026-10-25

`List.sort` of Strs adds five words an element: 76.3 MB at 2,000,000 Strs
(bench/results/sortstr-2026-10-24.md). `spec/std.md:326` promises the sort
is stable, so Go's in-place pdqsort is out; the two stable in-place sorts
were built and measured, Go's `sort.Stable` (SymMerge) and a bottom-up
merge with a half list of scratch. The first cuts the sort's scratch by 60
percent and costs 6 times the instructions, the second cuts it by 40
percent and costs 48 percent more. Both lose, so neither is landed; the
two findings below say why, and what would change the answer.

## How it was measured

Probe: `bench/perf/sort/xo` built with 2,000,000 keys ("key" and a number
below 100,000,000), a `gen` mode that builds the list and stops and a
`strs` mode that builds and sorts, three runs of each with `/usr/bin/time
-l` for max RSS and instructions retired. Same probe and shape as
bench/results/sortstr-2026-10-24.md. A = an xo at `origin/main`
(b7faeedb3, whose std is the base for this change), B = Go's
`sort.Stable` ported to `std/native/prelude.xo` (`insertionSort` in blocks
of 20, then `symMerge`/`rotate`, BSD attribution), C = a bottom-up merge
of the strings with a scratch of half the list. A, B and C build the probe
from the one source, each with its own xo, `XO_INLINE_STRICT=1`.

| build | gen max RSS | strs max RSS | the sort adds | strs instructions | against A |
|---|---|---|---|---|---|
| A, index sort (main) | 109.3 MB | 185.6 MB | 76.3 MB | 3,292,000,000 | 1.000 |
| B, `sort.Stable` | 109.3 MB | 139.8 MB | 30.5 MB | 19,970,000,000 | 6.07 |
| C, half buffer merge | 109.3 MB | 155.1 MB | 45.8 MB | 4,884,000,000 | 1.48 |

Every run printed the same checksum in all three builds (987883420 for
`strs`, 430194146 for `gen`), so the ports order the Strs identically.

## Why B costs six times: the swaps, not the comparisons

`XO_PROF=cpu XO_PROF_HZ=1000` on B, 321 samples (1796 ms of wall), the
`strs` mode:

| frame | flat |
|---|---|
| `str_swap` (a three statement swap of two Strs) | 36.1% |
| `drop Str` (the release every `list.set` does) | 21.8% |
| `str_sym_merge` (the comparison loop) | 13.1% |
| `platform_memcmp` (Str comparison) | 6.5% |
| `share Str`, `dropp Str`, `sharep Str` | 4.6% |

62 percent of B is reference counting inside the swaps and 20 percent the
comparisons, which matches SymMerge's shape: it does O((M+N) log M) swaps
and O(M log(N/M+1)) comparisons a merge, so it is the swaps (each a
retain and a release) that multiply out. That is the price of moving Strs:
an `Int` move in the index sort is a store, where a `Str` move is a store
and two reference counting operations.

C moves all the strings of each merge once into the scratch and back, pays
the same reference counting per move, and sorts with full `Str`
comparisons rather than the seven byte prefix `Int`s the index sort
compares, which is the 48 percent.

## The floor for a non-mutating sort

`spec/std.md:326`'s signature returns a new list, and `var a = xs` copies
the list when it is shared (`compiler/llvm/func.xo:1300`,
`compiler/arm64/calls.xo:170`), so any implementation that returns a new
list holds the receiver's element array (2n words) and a second one to
sort (2n). B's measured 30.5 MB is exactly that second array (2,000,000
Strs of 16 bytes). The 76.3 MB today is 5n: the receiver 2n, the prefix
list n, the index list n and its merge buffer n at the merge peak, and 2n
+ n + 2n at the gather. So the most a std change can save is 45.8 MB,
which is C, and it costs 48 percent of the instructions.

## What would change the answer

1. An in-place API: `List.sort` on a `var` receiver, or a new
   `sort_in_place(var xs: List[T])`, which is Go's `slices.Sort` shape.
   For a uniquely owned list the scratch is zero and for a shared one it
   is the copy above. It is a spec addition and a decision, and it keeps
   the stable promise.
2. A runtime intrinsic that moves a list's elements without reference
   counting: permuting a list's own elements never changes which strings
   are referenced, so `rt.rl_swap(l, i, j)` (or a 16 byte element
   compare-and-swap) is safe. C's moves would then be a few instructions
   instead of a retain and a release, and C projects to about the index
   sort's instructions at 3n. This is the same shape as the
   `rt.case_fold16` intrinsic: a runtime operation the source cannot
   express today.
3. Accept C as measured. It is 22 percent off the program's peak (185.6 to
   155.1 MB) for 48 percent more instructions, which is what the item's
   bar rules out.

## What is landed

A test-only change: the `prelude` corpus (Go's `sort.Strings` as the
oracle) covers `List.sort` of Strs for 0, 1, 2, 12, 13 and 50 keys, keys
already in order, reversed, all equal, keys that share a seven byte
prefix, and keys where one is a prefix of another, plus two tests for what
the spec promises: the receiver keeps its own order, and `sort_by` keeps
the input order of equal keys. SymMerge's 4n and the half buffer's 3n both
pass those rows, so the rows do not decide between them; they hold the
existing sort in place.
