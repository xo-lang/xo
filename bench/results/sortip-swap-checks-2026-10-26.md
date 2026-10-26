# A swap's two index checks: the price in sort_in_place (2026-10-26)

`rt.list_swap` and `rt.list_swap2` now check each index against the list it
indexes (commit be41164b8). Before, the SSA builder gave the call no source
position, so `bounds_check` returned early and neither index was checked on
any native backend; and once it ran, it read the index and the list off the
operation's own arguments, so the second index was checked as the first.

This measures what the checks cost, on the one std path where they are hot
and cannot be proven away: `sort_in_place`'s merge loop
(`std/native/prelude.xo` merge_runs_in_place, the swaps at 1549, 1552, 1558
and the insertion pass at 1603). Its indexes are `lo`, `hi`, `w`, `j` and
`left`, all bounded by the caller's `hi <= xs.len()` clamp, which is not a
fact the callee can see, and `Proof` records only the true edge of a branch,
so no index is provable and every swap carries two checks.

## xo bench --ab, 10 rounds, quiet window

A is `bench/perf/sortip` built by the parent's xo and std (d0713c36f, the
branch base); B is the same program built by this change. `gen_*` builds
the 2,000,000 element list and reads it, `ip_*` runs `sort_in_place` on it,
so the difference between the two rows of a kind is the sort.

| run | instructions A | instructions B | B/A | wall A (ms) | wall B (ms) | B/A |
| --- | --- | --- | --- | --- | --- | --- |
| gen_ints | 53,166,689 | 53,182,392 | 1.000 | 6.43 | 6.24 | 0.97 (noise) |
| ip_ints | 1,120,285,132 | 1,274,193,528 | 1.137 | 143.6 | 148.1 | 1.031 |
| gen_strs | 633,078,078 | 633,012,049 | 1.000 | 32.07 | 32.11 | 1.001 (noise) |
| ip_strs | 3,728,929,461 | 3,936,257,625 | 1.056 | 493.7 | 498.4 | 1.010 |
| gen_u8 | 43,360,390 | 43,351,378 | 1.000 | 5.51 | 5.46 | 0.99 (noise) |
| ip_u8 | 1,111,455,850 | 1,265,283,513 | 1.138 | 40.45 | 45.96 | 1.136 |

The `gen_*` rows are the control: the list build is untouched, 1.000 of the
instructions, so the whole difference is the sort. The sort pays 5.6 to
13.8 percent more instructions and 1.0 to 13.6 percent more wall on this
machine; the instruction counts are exact (spread 0.02 percent) and are the
number to trust, the wall carries the machine's noise (the load was 8 to 9
during the window, kept for all rows).

About 3.6 instructions a swap: a load of the list's length (the swap path
itself never loaded it, the address came from elem_addr), the `cmp` and the
`b.hs` to the fault label. The count of `b.hs` in the two binaries is 296
before and 326 after (+30, the swaps of the merged prelude plus
std/internal's probes).

## bench/perf instruction sweep, the same 34 programs

Parent d0713c36f against this change: 6,894,662 to 6,895,182 instructions
(+520, +0.008 percent), check branches 18,255 to 18,266 (+11). Three
programs carry the cost, aesgcm, chacha and rsa, +474 each and +26 checks,
because they link std/internal, whose list_swap probes now check their
constant indexes; `text` (-578, -40 checks) and `json` (-324, -27) are the
div_of_len rule of the parent commit, which is worth more than the swap
checks cost everywhere except those three.

xo's own native `__text` is unchanged: the fixed point from the parent's
seeds gives s1 = s2 = s3 (sha256
daff0c7a9b8d7acec5c1731f1bccea7487e1c4b0e5a1483acf24b026870d0292), 60,975,208
bytes, so xo has no swap in a hot path of its own.

## What would get it back, in order

1. `bounds_check` loads the list's length for the compare even when the same
   list's length is already in a register: `same_loaded`-style memos exist
   for other loads, not for `list_len`. One instruction a check, and the
   sort's two checks a swap then cost two.
2. Prove merge_runs_in_place's indexes. `Proof` records only a branch's true
   edge, so the caller's clamp (`if hi > n { hi = n }`) leaves nothing
   behind; recording `n <= hi` on the fall through of a guard whose true
   edge returns would make `hi <= xs.len()` a fact, and `w < hi` plus
   `j < hi` then follow from the loops' own counters.
3. Accept the cost: the alternative was a swap that could store past the end
   of a list, which the parent did silently.
