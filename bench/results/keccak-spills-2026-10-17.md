# Keccak's register spills on arm64: measured, parked (2026-10-17, xo-fa)

Question: is it worth removing the stack traffic of
std/native/llrt/sha3.xo keccak (the lead's queue, for SHA-3 in ML-KEM in
TLS)?

## Where the spills are

`xo asm --backend=arm64` of bench/perf/tls (main da7292a2), function
sha3.keccak: 1063 lines, 306 stack accesses, 248 of them in the loop body
(four rounds; the renaming of lanes repeats every four), so about 62 per
round. Live in a round: the 25 lanes and the C, D and B temporaries
against about 26 allocatable registers (x0 to x15, x19 to x28; x16 and
x17 are the reload scratch). The floor with that pressure is about 42
per round: each spilled lane loaded for C, loaded again for B, stored
once. At most about 20 memory operations of about 210 instructions per
round can go: about 10% of keccak.

## Keccak's share of a handshake

A scratch std whose keccak runs every permutation twice (on a copy, then
on the state) against main, bench/perf/tls/measure.sh, N=200, client
instructions (best of 5):

| workload | main | keccak twice | delta |
|---|---|---|---|
| x25519 (with ML-KEM) | 1711611160 | 1757846788 | 46.2M |
| x25519only | 1494941237 | 1496345492 | 1.4M |

So keccak is about 2.7% of the x25519 client's instructions and about
21% of ML-KEM's 216.7M (x25519 minus x25519only). Removing the extra
spills would save about 0.3% of a handshake.

A CPU profile of the same client (XO_PROF=cpu, XO_PROF_HZ=1000, N=400)
agrees: mlkem.hybrid_share_for is 6 of 513 ms. The client is sendto
(48% flat) and nistec: mul4p 13% flat, pt_mul 22% cumulative.

## Decision

Parked. Larger targets in the same workload, in order: nistec.mul4p
(the P-256 field multiplication), the rest of ML-KEM (NTT, sampling),
then these spills. What would remove them when it is their turn: a
schedule of the generated Xo source that computes C from memory and
writes each row's χ straight back to the state array (75 memory
operations a round, but no reloads through x16), or allocating x30
(the link register, free in a loop with no calls) for one more lane.
