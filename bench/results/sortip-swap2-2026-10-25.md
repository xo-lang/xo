# Step 3b, the pair-swap merge, against List.sort and step 2, 2026-10-25

ADR 0374's step 3b removes the reference counting from the in-place sort's
moves: the merge swaps the left run into a placeholder scratch and swaps
every element of the merge back (`rt.list_swap2` across the list and the
scratch, `rt.list_swap` inside the list), so an element move is two loads
and two stores and never a retain or a release.

## How it was measured

The same probe and method as bench/results/sortip-2026-10-25.md:
`bench/perf/sortip/xo` built with the tree's `xo` at `XO_INLINE_STRICT=1`,
arm64, 2,000,000 elements, three runs a mode with `/usr/bin/time -l` and the
median taken; the sort's cost is the sorted run's numbers minus its own
`gen` run's. The `sort` and step 2 columns are that file's numbers, from the
same probe on the same machine. Wall is indicative only (load average 8.9
to 13 during the run).

| 2M elements | sort | step 2 (counted moves) | step 3b (pair swaps) | 3b / sort | 3b / step 2 |
|---|---|---|---|---|---|
| Ints instructions | 1,008,600,000 | 1,129,300,000 | 1,068,000,000 | 1.06 | 0.95 |
| Ints extra RSS | 16.0 MB | 14.3 MB | 8.0 MB | 0.50 | 0.56 |
| Ints wall | 0.13 s | 0.17 s | 0.14 s | 1.08 | 0.82 |
| Strs instructions | 2,636,400,000 | 3,744,400,000 | 3,098,900,000 | 1.18 | 0.83 |
| Strs extra RSS | 80.0 MB | 22.7 MB | 16.0 MB | 0.20 | 0.70 |
| Strs wall | 0.72 s | 0.95 s | 0.54 s | 0.75 | 0.57 |
| UInt8 instructions | 998,200,000 | 1,176,900,000 | 1,068,300,000 | 1.07 | 0.91 |
| UInt8 extra RSS | 2.0 MB | 1.7 MB | 1.1 MB | 0.55 | 0.65 |
| UInt8 wall | 0.04 s | 0.06 s | 0.04 s | 1.00 | 0.67 |

## What the numbers say

- **The pairing wins on all three element kinds, so no per-type merge choice
  is needed.** The design expected the swap merge to lose a few percent on
  Ints and UInt8, where a counted move counts nothing; it is 5 to 9 percent
  cheaper there instead (0.95 and 0.91 of step 2), because step 2's move
  still pays the element store's drop and retain calls and its bounds checks
  on both sides, while a swap is two loads and two stores. On Strs, where a
  move counts and releases, it is 0.83 of step 2 and now 1.18 times `sort`,
  down from 1.42.
- **The scratch reservation helps memory as well.** `List.with_capacity`
  sizes the scratch to the widest left run once (1,048,576 slots at 2M
  elements, 52 percent), so it never doubles by growth: extra RSS goes 22.7
  to 16.0 MB on Strs (0.20 of `sort`) and 14.3 to 8.0 MB on Ints. The
  measured 16.0 MB for a 16 byte Strs slot area is the reservation itself.
- **The init is counted once and is visible in these numbers.** The scratch
  starts as copies of `xs[0]`, one counted retain a slot (1,048,576 of them
  at 2M elements, released when the scratch is dropped), which is about one
  fortieth of the ~42M counted moves step 2 pays; each merge then only swaps
  the copies it needs out of the way.
- Ints and UInt8 are still 5 to 7 percent above `sort`'s instructions: that
  residue is the placeholder traffic the half-buffer shape needs (a swap a
  move, both ways) and the `gen` mode's own build, not counting.

## The keyed forms

Second pass, same method and binary, `sortb` and `ipb` in one run (load
11.4 to 12.7 at the end). `sort_in_place_by` sorts an index list in place
(with pair swaps, since it is a `List[Int]`) and then rotates the cycles of
the permutation with `rt.list_swap` transpositions, one swap a cycle step
and no counted write, so the keyed form no longer counts anything either.

| 2M elements | sort_by instructions | step 2 | step 3b | 3b / sort_by | 3b / step 2 |
|---|---|---|---|---|---|
| Ints | 1,587,500,000 | 1,770,400,000 | 1,719,200,000 | 1.08 | 0.97 |
| Strs | 3,475,600,000 | 3,794,400,000 | 3,707,200,000 | 1.07 | 0.98 |
| UInt8 | 1,578,700,000 | 1,765,400,000 | 1,716,700,000 | 1.09 | 0.97 |

Extra RSS is unchanged from step 2 (0.86, 0.89 and 0.81 of `sort_by`'s),
since the keyed form's arrays are the keys and the index list, not the
scratch. Wall is 0.39 against 0.22 s on Ints, **0.80 against 0.76 on Strs**
(0.97 s with step 2's held-element permutation, so the swap rotation pays
most where an element is counted) and 0.12 against 0.06 on UInt8; the
remaining Ints and UInt8 wall gap has the same cause step 2's row gave, the
permutation walking the receiver in cycle order while `sort_by` writes its
output sequentially. Checksums match `sortb`'s (901184094, 987883420,
838940239).

The transposition rotation was measured as its own step, in this binary:
against the held-element rotation in the same probe, Strs instructions go
3,742.8M to 3,707.2M (0.99) and wall 0.97 to 0.80 s, while Ints and UInt8
stay within half a percent (1.075 to 1.083 and 1.081 to 1.087 of `sortb`,
timer noise at load 11 to 12). It is kept: it removes the keyed form's last
counted moves, and it is the reason the Strs wall row improves.
