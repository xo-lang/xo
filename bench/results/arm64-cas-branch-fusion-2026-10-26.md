# arm64 compare and swap branch fusion (negative result)

Date: 2026-10-26. Xo Lang 1. Machine: the development Mac (arm64).
Measure-class `xo bench --ab` (concurrency: wall and cycles decide;
callgrind serializes threads). A is the parent's xo (16e3ea0dd), the other
side a variant built by one changed xo. The branch is kept as
`refs/recover/lang1-cas-fusion` (63e5153ae), not on main.

## Question

x86_64 branches on cmpxchg's ZF since 957f31aaf. On darwin arm64,
`rt.slot_cas` lowers to `mov; cas; cmp; cset eq`, and a branch on its Bool
then tests the register again (`cbnz`, or `cbz` when negated). Does
branching on the compare itself make the runtime's locks faster?

## Variants

- B: calls.xo emits the `cas` and its `cmp; cset` as two instructions, so
  opt.xo thread_cset fuses `cmp; cset; cbnz` into `cmp; b.eq`; and
  thread_cset's arm64 fusion also takes `cbz`, branching on the inverse
  condition (about 190 runtime functions change by 1 to 3 instructions).
- C: the CAS split only (thread_cset unchanged).
- D: the `cbz` fusion only (no CAS split).

## Results

| row | B / A wall | C / A wall | D / A wall |
|---|---|---|---|
| atomic cas | 1.008 (noise) | | 1.011 (noise) |
| atomic flag | 1.026 (noise) | | 1.005 (noise) |
| atomic contended | 0.954 (noise) | | 0.999 (noise) |
| ctx mutex | 1.016 (1.012 to 1.025) worse | 1.018 (1.008 to 1.023) worse | 0.997 (noise) |
| ctx spawn | 1.013 (noise) | | 0.977 (noise) |
| ctx await | 0.926 (noise) | | 1.000 (noise) |
| httpclient len | 0.994 (noise) | | 1.002 (noise) |
| httpclient h2len | 1.004 (noise) | | 1.011 (1.000 to 1.023) worse |

ctx mutex B reproduced at 40 rounds in both orders: A/B wall 1.016, cycles
1.012; B/A wall 0.984, cycles 0.987; A/A 0.999 and 1.000. Instructions
were 0.996 of A in both B and C. C/B (the `cbz` fusion added to C):
cycles 0.989 with the same instruction count.

## Cause, and what is not known

`mutex_lock`'s only change in B is the intended one: `cset x10, eq; cbnz
x10` became `b.eq`. So the 2% more cycles in C come from the fused branch
right after the `cas` itself, or from layout. The likeliest reading is
that a failed lock attempt now retries sooner, which adds cache line
traffic in a contended mutex, so fewer instructions run slower. That is
a hypothesis; no run here isolates it. D's 190 small changes gain nothing
measurable, and h2len is at the edge of worse.

## Next steps, ranked

1. Test the retry hypothesis directly: B with a spin hint (`yield` or
   `isb`) on the CAS failure path of cabi.mutex_lock only, ctx mutex
   --ab against A. If it wins, the fused branch is fine and the lock's
   backoff is what to tune (Lang 3's spin work, decision on adaptive
   spin).
2. Fuse only an uncontended CAS: a CAS the runtime retries in a loop
   keeps the cset; one tested once (try_lock, flags) takes b.eq. Measure
   atomic flag and ctx spawn, the uncontended rows.
3. Leave arm64 as it is: the branch costs one instruction an
   uncontended lock, below what these benches resolve.
