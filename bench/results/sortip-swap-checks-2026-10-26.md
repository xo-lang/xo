# The swap index checks, and the guard fact the bounds proof reads (2026-10-26)

Two changes meet in this file. The first landed (`be41164b8`): `rt.list_swap`
and `rt.list_swap2` check each index against the list it indexes, and the
measurement below is what those checks cost on the one std path where they
are hot. The second is this change: the arm64 bounds proof now reads the
fact a guard leaves, so two of the checks the merge paid are gone.

## Part 1: a swap's two index checks, the price in sort_in_place

`rt.list_swap` and `rt.list_swap2` now check each index against the list it
indexes (commit be41164b8). Before, the SSA builder gave the call no source
position, so `bounds_check` returned early and neither index was checked on
any native backend; and once it ran, it read the index and the list off the
operation's own arguments, so the second index was checked as the first.

This measures what the checks cost, on the one std path where they are hot
and cannot be proven away: `sort_in_place`'s merge loop
(`std/native/prelude.xo` merge_runs_in_place). Its indexes are `lo`, `hi`,
`w`, `j` and `left`, all bounded by the caller's `hi <= xs.len()` clamp,
which was not a fact the callee could see, and `Proof` recorded only the
true edge of a branch, so no index was provable and every swap carried two
checks.

### xo bench --ab, 10 rounds, quiet window

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

### bench/perf instruction sweep, the same 34 programs

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

### What would get it back, in order

1. `bounds_check` loads the list's length for the compare even when the same
   list's length is already in a register: `same_loaded`-style memos exist
   for other loads, not for `list_len`. One instruction a check, and the
   sort's two checks a swap then cost two.
2. Prove merge_runs_in_place's indexes. `Proof` records only a branch's true
   edge, so the caller's clamp (`if hi > n { hi = n }`) leaves nothing
   behind; recording the guard's other edge would make `hi <= xs.len()` a
   fact, and `w < hi` plus `j < hi` then follow from the loops' own counters.
   This is the change in part 2.
3. Accept the cost: the alternative was a swap that could store past the end
   of a list, which the parent did silently.

## Part 2: the guard fact removes two checks

2026-10-26. The arm64 bounds proof gained the fact a guard's other edge
leaves (`if hi > xs.len()` past a fault leaves `hi <= xs.len()`), the same
fact carried through a clamp's phi (`if h > xs.len() { h = xs.len() }`),
and both applied to a joined index (a block parameter whose every incoming
value is proven). std's merge (`merge_runs_in_place`, the sort in place of
decision 0374) says the length once per merge, faults on a broken caller
instead of silently returning (`rt.fault_raise` with an empty location, the
message naming the function), and tests the two cursors with a break
instead of `&&`, so both cursors are bounded by a comparison the proof
reads.

### Static check counts, arm64, 2,000,000 elements

`xo dev dump asm bench/perf/sortip/xo`, built by the base commit's xo and by
the change's: 201,209 to 201,456 lines and 418 to 412 `b.hs` bounds-check
branches (-6: one index check leaves each of `merge_runs_in_place[Int]`,
`[Str]` and `[UInt8]`, once per monomorphization and once per
specialization), `bl` 10,188 to 10,218 (the fault path's call).

The static count hides the win: the check that leaves is inside the merge
loop, which runs once per element moved.

### xo bench --ab, 10 rounds

Run through `HEAVY_CLASS=measure ~/xo-lead/agents/heavy.sh ds1-ab-measure2`,
whose log records `start (measure, window until 16:08:10)`, load 3.21 (A)
and 3.21 (B) median, no round loaded. A is the base commit's xo
(2bd010b71) with its own std, B this change's; same source. An earlier
attempt (ds1-ab-measure) used a stale A binary from the scratch directory
and is not quoted.

| mode | metric | A | B | B/A | 95% interval | verdict |
| --- | --- | --- | --- | --- | --- | --- |
| ip_ints | instructions | 1,122,611,353 | 1,020,403,770 | 0.909 | 0.908 to 0.911 | better |
| ip_ints | wall_ms | 147.2 | 141.5 | 0.961 | 0.947 to 0.971 | better |
| ip_ints | cycles | 572,681,014 | 549,055,656 | 0.959 | 0.951 to 0.963 | better |
| ip_ints | ipc | 1.961 | 1.861 | 0.949 | 0.944 to 0.954 | worse |
| ip_ints | max_rss_kb | 26,456 | 26,488 | 1.001 | 0.999 to 1.002 | within noise |
| ip_strs | instructions | 3,740,848,781 | 3,640,763,372 | 0.973 | 0.972 to 0.974 | better |
| ip_strs | wall_ms | 503.8 | 505.7 | 1.004 | 0.971 to 1.012 | within noise |
| ip_strs | cycles | 2,013,763,111 | 2,014,441,953 | 1.0 | 0.986 to 1.007 | within noise |

B in that run was the three-Map shape of the proof. The one-map refactor
changed xo's own code size, not the sort code the backend emits from the
same sources, so the ratio carries over; the fresh window run against
current main's xo is quoted below when it lands, and the two numbers to
compare are the instruction ratios, not the wall.


### The sweep over bench/perf

All 38 programs of bench/perf with a `main.xo`, built by the base commit's
xo (same std, same compiler but for the change) and by the change's: 7,794,723
to 7,794,660 instructions (-63), bounds-check branches 20,710 to 20,700
(-10: 6 in sortip, 2 each in httpclient and tls). No program grew. The sweep
ran before the guard body became a fault; the fault sits on the true edge,
which no workload takes, and replaces a `return` with a branch to a block
after the loop, so the taken path's compare and branch are unchanged.

### What still pays a check

In the merge, `w` (the slot the next element goes to) and the `lo + k` of
the placeholder loop keep their checks: their bounds are loop invariants
(`w <= hi` follows from the two cursors' progress), not comparison facts, so
the proof does not reach them. An invariant range pass would be the next
step; it is a separate change.

The four checks the merge had before, and after: `j` (bounds `xs`), `i`
(bounds `scratch`) and one `w` (the second `w` was already shared through
the checked memo) went from 3 to 2 per monomorphization; `w` and `lo + k`
remain.

### xo's own code size, and the shape of the fact storage

sizebreak's `__text` of the fixed point's s2, built by the base commit's xo
(2bd010b71, `/tmp/dsbase/bin/xo`) in a scratch worktree at that commit with
one file set per variant:

| variant | s2 __text | delta |
| --- | --- | --- |
| base | 61,190,696 | |
| fieldlicm.xo only, three extra Map fields in Proof | 61,406,032 | +215,336 |
| fieldlicm.xo only, one `facts` map (the shape committed here) | 61,199,392 | +8,696 |
| prelude.xo only, the guard with a plain `return` | 61,190,696 | 0 |
| the whole change (this commit's fixed point s2) | 61,199,392 | +8,696 |
| current main fddc2ea91, its own fixed point s2 | 61,667,804 | |
| the rebased change on fddc2ea91, its fixed point s2 | 61,676,612 | +8,808 |

The first shape kept `le`, `lens` and `unstable` as separate Map fields of
Proof, and Proof is a field of G: every copy or drop of a G then retained
and released three more heap fields inline, in every method that has one.
The growth was spread over about 50 arm64 functions that never call the
proof (arm64.G.emit +11,432 B, its disassembly +2,858 instructions with the
same `bl` count, +765 branches, and seh_end +786 instructions that are 60
retain and release triples, one per G copy in it). Holding every fact in
one map under a key per kind (`fact_key`, kinds LT, LE, LEN, UNSTABLE) puts
Proof's heap fields back to two, idom and facts, and the cost falls to
+8,696 B (+0.014 percent of xo's own text). The trap is recorded in
knowledgebase/gotchas/struct-heap-field-count-code-size.md.

The same effect on amd64: `xo asm --backend=amd64` of bench/perf/sortip
gives 783 to 777 `jae` (the six merge bodies lose one each), and inside
merge_runs_in_place's own body the check goes 3 to 2 (Int), 4 to 3 (Str)
and 3 to 2 (UInt8), with `jb` and `ja` unchanged.

The 39 programs of bench/perf with a `main.xo` (four have no xo.mod and are
skipped) built by both xos: total `__text` 23,550,600 to 23,550,448
(-152 B), httpclient and tls each -128 B (the checks they lose, from the
sweep above) and sortip +104 B (its six merge bodies lose six checks and
gain the one cold fault body), every other program identical.

## Rerun on the rebased tree

The commit was rebased onto fddc2ea91 (current main, which carries the swap
checks and the div_of_len rule). On that tree: `xo dev codegen
--backend=arm64,llvm test/codegen` gives 18 program builds and 213
patterns, 18 pass, 0 fail; arm64 TestGoldenRun/list_sort_guard,
half_index and list_swap_guard pass, as do LLVM TestGolden and
TestGoldenGoParity for the same three; `xo test compiler/arm64` is 54
passed, 0 failed; the fixed point gives s2 = s3 in both assembly
(749,518,476 bytes) and binaries (107,625,167 bytes), and s3 builds and
runs hello; the s2 `__text` beside main's own fixed point is in the table
above (+8,808).
