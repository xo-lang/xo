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

## sub_p256 after the restart (same day, parked)

A larger instruction limit for leaves (INLINE_LEAF_INSTRS 80 against
40) does not make nistec.sub_p256 inline: bench/perf/tls still has its
36 calls. Its limit is the block count, not the instructions: each of
its nine inlined calls (four cpu.adc, four cpu.adc_carry, ct.mask_bit)
adds a continuation block, about 19 against INLINE_MAX_BLOCKS 12, and
discounting those blocks cost more than it gained
(arm64-inline-2026-10-15.md, variants A to C). Not landed. What would
make it inline: cpu.adc and cpu.adc_carry as SSA intrinsics (no call,
no continuation block; the emitter's adcs chain already fuses them), or
Xo Lang 2's explicit adc chain helper. The sweep driver of these runs is
now bench/perf/sweep.py (each bench/perf/<dir>/measure.sh with two xo
binaries in alternating rounds).


## sub_p256 inlines (2026-10-17, xo-fa)

The parked section's diagnosis was wrong. cpu.adc and cpu.adc_carry were
already SSA intrinsics (ssa call.xo cpu_call: rt.u64_adc and
rt.u64_adc_carry, decision 0231), so no continuation blocks came from
them. What kept sub_p256 a call was arm64 inline.xo's inline_intrinsic:
its list of rt intrinsics that are instructions lacked the carry ones,
so sub_p256 was not a leaf and paid the non-leaf limits. With
rt.u64_adc, rt.u64_adc_carry, rt.u64_carry and rt.u64_borrow in it, the
calls of sub_p256 in bench/perf/tls go from 36 to 1.

Sweep (bench/perf/sweep.py, base its merge base, 114 workloads): 0
slower, 1 faster beyond spread (httpclient/len 0.991). P-256 within
spread: tls/p256 0.998, tls/srv-p256 0.996, tls/srv-p256-0 0.993; the
call was cheap next to the field multiplications around it.

## Carry pairs count once; one instruction constants (2026-10-17, xo-4)

Callgrind on linux/arm64 (bench/perf/tls, p256 handshakes, after
72bb4214) put arm64 at 9.55M instructions a handshake and LLVM at 7.30M;
nistec.add4p and sub4p, calls 29,000 and 13,000 times a handshake, were
0.78M of the gap. Their SSA cost was 47 and 41 (INLINE_MAX_INSTRS 40):

- an rt.u64_adc and an rt.u64_adc_carry of the same operands are one
  adcs but counted two. inline.xo adc_pairs counts each pair once,
  keying constants by value (every use of a literal is its own Const);
- a negative literal is a Neg of a Const and counted as an instruction;
  it folds (fold_value), so it counts as a constant.

add4p and sub4p cost 38 each and inline at every site (70 and 30 calls
to 0). x25519.fe64_add, fe64_sub and cswap stay calls: their loops are
17 to 25 blocks (INLINE_MAX_BLOCKS 12).

util.xo mov_imm puts a logical immediate (one orr, as `mov $d0, #N`;
-2^32 was a movn and a movk) and a 32 bit movn (`mov $wd0, #N`, zero
extended) in one instruction: 98 movk fewer in the TLS client.

Sweep (bench/perf/sweep.py, base main's compiler with the same std, 114
workloads, 3 rounds): 0 slower, 19 faster beyond spread, among them
json/parse 0.966, tls/p256 0.975, tls/x25519only 0.975, tls/tls12 0.979,
tls/x25519 0.978, portable/u64 0.981, text/quote 0.981, tls/rsa2048
0.984, tls/ed25519 0.985. Mac cycles (bench/perf/tls, median of 7):
p256 556.1 to 540.8 M, x25519only 418.9 to 413.7 M, tls12 511.7 to
502.7 M.

## adc chains an instruction per pair (2026-10-17, xo-4)

nistec.mul4p saved 5 callee saved pairs on arm64 (LLVM 1): an adc
chain was one template whose sums could share no register with its
operands, and whose operands had to be defined before it, so the
generator made a row's 8 products first. selfhost/arm64 calls.xo
ADC_SPLIT makes each pair its own adds or adcs (keep), the carry in the
flags (gotchas/arm64-carry-flag-between-instructions.md), and a product
read only by its pair is made right before it (chain_products; false on
x86, whose imul writes the flags). scripts/nistecmul4p.py makes each
product before its pair and ends a chain whose carry is added on with
an adcs of xzr. mul4p 166 to 154 instructions, 1 callee saved pair, no
spill; sqr4p 143 to 135, 5 pairs to 1.

Mac, bench/perf/tls (min of 7 instructions, median cycles), main's
compiler and std against the branch's:

| workload   | arm64 Minstr     | arm64 Mcycles  | LLVM Minstr      |
|------------|------------------|----------------|------------------|
| p256       | 1934.1 to 1873.4 | 558.4 to 548.1 | 1620.8 to 1613.2 |
| x25519only | 1440.1 to 1392.5 | 422.7 to 418.2 | 1201.4 to 1195.7 |
| tls12      | 1731.8 to 1674.8 | 517.6 to 514.3 | 1443.6 to 1437.7 |

Sweep of the compiler alone (bench/perf/sweep.py, both sides the
branch's std, 114 workloads, 3 rounds): 0 slower, 11 faster beyond
spread, tls/x25519only 0.908, tls/p256 0.910, tls/x25519 0.917,
tls/tls12 0.918, tls/rsa2048 0.934, tls/ed25519 0.940 (the old
compiler on the new mul4p, whose products come between the pairs,
breaks its chains: these ratios are the new std's dependence on the
split, not the gain over main).

## x25519's field product with lazy products (2026-10-17, xo-4)

scripts/x25519fe64.py makes each product right before its pair (as
nistecmul4p.py), folds the carry-adds into the chains (add_row, and
reduce8's top word), and calls.xo chain_product takes a constant
operand (the reduction's 38: a mov, no flags). x25519.fe64_mul 123 to
113 instructions, callee saved pairs 7 to 3 (2 spill moves); fe64_sq
88 to 86, 5 to 4. Mac bench/perf/tls, min of 7: x25519only 1391.6 to
1385.8 M instructions, p256 1871.4 to 1864.6, tls12 1674.8 to 1662.2;
cycles within noise (406.6 to 407.1, 521.6 to 529.4, 519.0 to 512.5).
TestX25519Diff and TestNistecDiff pass on both backends.

## Inline copies and sbcs in chains (2026-10-17, xo-4)

Linux callgrind (bench/perf/tls p256, after 37c6ed8d): arm64 8.80M
instructions a handshake, LLVM 7.30M; nistec 857K over LLVM, libc's
memcpy, memset and memmove 242K. calls.xo: rt.mem_copy of a known length
up to 128 bytes is ldp and stp (nistec's point functions copied 32 bytes
through memcpy); a chain pair adding ~b (a Not, or -1) is sbcs of b
(subs for a first pair with carry in 1); two Nots of one operand pair
(same_arg; the inliner's pair key too); a first pair adding 1 to 4095
is adds #k. nistec.sub4p 42 to 19 instructions, pt_add256 1349 to 1047,
pt_double256 875 to 686.

Mac bench/perf/tls, min of 7 runs (main 37c6ed8d's compiler against the
branch's): p256 1877.0 to 1650.5 M instructions (-12.1%), tls12 1672.2
to 1474.3 (-11.8%), x25519only 1392.6 to 1318.2 (-5.3%); median cycles
867.9 to 814.7, 619.6 to 590.8, 662.9 to 650.6 (a loaded machine).
LLVM p256: about 1613.

## Rotated operands and a lazy-rotation Keccak (2026-10-17, xo-4)

isel.xo rot_operand: a rotation by a constant read once by eor, orr,
and, bic, orn or eon is that instruction's rotated register operand
(logic_ror; false on x86 when the amd64 retarget lands).
scripts/keccaklazy.py generates sha3.keccak with lazy rotations (XKCP's
ARMv8 form): a lane is a value and a pending rotation, applied through
the operands that read it, and the 25 pending rotations at the 4-round
body's end. arm64 keccak 906 to 829 instructions, ror 116 to 25; its
stack accesses 261 to 288 (the spills are the next lever: about 72 a
round against a floor near 42, bench/results/keccak-spills-2026-10-17.md).

Mac bench/perf/tls, min of 7: x25519 (with ML-KEM) 1395.7 to 1392.0 M
instructions; x25519only 1310.6 to 1310.5. Sweep (digest, tls, json,
text; 59 workloads, 3 rounds): 0 slower, 0 faster beyond spread.
TestMLKEMAccumulated (10,000 iterations, both backends) passes.
