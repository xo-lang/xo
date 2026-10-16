# The arm64 inliner's cost: constants and cold slow paths (2026-10-16)

xo-fa, the lead's item (c) after decision 0239's inlined frames. Base:
main at 4c35fdec (v0.18.0) for A to C, b8eb3363 for D (both sides
rebuilt on it); the whole bench/perf set on arm64 (151 workloads, the
sweep's phases single, testworld, conc, httpclient, tls, http; 3
interleaved repetitions, medians, instructions retired; "beyond spread"
means the ratio of medians is outside the larger max/min of either
side).

## Question

selfhost/arm64 inline.xo counted every SSA value of a callee toward
INLINE_MAX_INSTRS (40), constants included, though a constant is an
immediate or one shared mov. Xo Lang 2's nistec.sub_p256 (limb
constants) stayed a call. Does not counting constants pay?

## Variants

| Variant | Rule | Slower beyond spread | Faster beyond spread | Worst |
|---|---|---|---|---|
| A | constants never counted | 12 | 14 | digest hmac 1.068, pbkdf2 1.055, sha256_small 1.048, tls chacha 1.032 |
| B | A only in one-block functions | 18 | 14 | digest hmac 1.076, pbkdf2 1.059 |
| C | A, and the runtime's cold slow paths (LLVM's list, now ssa.cold_fn) never inlined | 5 | 20 | sysio chan 1.019 (7 reruns: 1.019, beyond its 1.015) |
| D (landed) | C, but constants free only in leaves (calls only inline rt intrinsics and inlined leaves) | 1 | 13 | fsproc remove 1.001 |

What went wrong in A and B: the sha256 code did not change; the
runtime's paths did. obj.push_slow (one of LLVM's cold functions)
inlined into obj.rs_push_word (31 to 128 instructions, stack slots 0
to 2), so every Bytes push paid for it. In C, free constants let the
scheduler's pool.run_item and pool.steal_round into pool.park_wait (343
to 714 instructions, stack slots 5 to 68): sysio chan 1.019.

D's faster rows: atomic contended 0.915 (spread 1.057), ctx budget
0.922 (budget.tick_slow no longer inlined into budget.tick, 99 to 36
instructions), ctx mutex 0.973, text float_fmt 0.973, json enc_floats
0.974, text int_fmt 0.982, ctx defer 0.985, text debug 0.988, alloc
tasks 0.989, json enc_small 0.990, sysio chan 0.998, atomic flag and
set 0.999 and 0.998. A's text and unicode gains from constants in
non-leaves (runecase 0.938, classify 0.961) are given up with them.

## What it did not do

nistec.sub_p256 is still a call (36 sites in bench/perf/tls in both
builds): its cost without constants (eight cpu.adc copies, the mask,
four stores) is still over 40, and with 36 sites the few-sites rule
does not apply. Next for it: report inline_walk's cost of sub_p256 and
decide between a larger limit for leaves (with a sweep) and the
explicit adc-chain helper Xo Lang 2 suggested.

Gates for D: spill and traceback tests, macho crypto/TLS/concurrency/
fault goldens, arm64 goldens, devgoldens, xo built by the new xo
building a program, the fixed point, -short.
