# crypto AES block, CBC and CTR against Go's crypto/aes and crypto/cipher (2026-11-11)

Decision 0605's timing row (the owner's rule: every std port carries one).
bench/perf/aesmodes and its Go twin (twin.go.txt), AES-128: block
(200,000 single block encryptions), cbcenc and cbcdec (a 64 KiB message
200 times), ctr (a 64 KiB message 200 times, in calls of 1,000 bytes);
`none` is the setup alone. twincheck agrees on all 5 workloads.

`xo bench --ab --rounds=10`, A the Go twin (go1.26.1), B the Xo program
(native, --release, cross-built on the Mac for linux/arm64 from
96502f843 plus this change), on the linux/arm64 offload host (host 1,
exclusive), with the 0.48.0 seed's xo; Xo / Go:

| workload | Go wall | Xo wall | wall (95%) | max RSS |
|---|---|---|---|---|
| none | 0.716 ms | 0.481 ms | 0.672 (0.626 to 0.724) | 0.19 |
| block | 5.859 ms | 6.296 ms | 1.074 (1.036 to 1.798) | 0.137 |
| cbcenc | 9.708 ms | 10.95 ms | 1.128 (1.109 to 1.141) | 0.113 |
| cbcdec | 6.132 ms | 4.506 ms | 0.735 (0.714 to 0.755) | 0.107 |
| ctr | 3.182 ms | 4.693 ms | 1.475 (1.326 to 1.497) | 0.107 |

Instructions: not reported on Linux.

The first build ran cbcenc and ctr at 1.93 and 1.93: CBC encryption
encrypted four blocks (encrypt4) for each block it needed and made a new
eight-word List per block, and CTR made the key stream as its own Bytes
per call. They now use one List per call, stored and loaded by address
(the GCM path's pattern), one block of AES for each CBC block, and
`Cipher.ctr_xor`, which XORs the data as the key stream is made.

What is left: every block's `cpu.aes_enc10` loads the 11 round keys
again, where Go's cbc_arm64.s and ctr_arm64.s keep them in registers
for the whole message and CTR runs 8 blocks interleaved. The next
experiment is an intrinsic over a run of blocks with the keys loaded
once (cpu.aes_enc10_n(st, kp, n), compiler/arm64), measured with
`--ab` on this bench.

## CTR through the CPU kernel (2026-11-11)

Ctr.xor's whole 64 byte chunks now run in the CTR kernel AES-GCM
already uses (rt.aes_ctr10/12/14: the round keys in registers, four
blocks interleaved), when the counter's low 32 bits do not wrap in the
call (the kernel steps those bits only). Same host and recipe, B built
from branch lang3-aesn:

| workload | Go wall | Xo wall | wall (95%) | before |
|---|---|---|---|---|
| ctr | 3.398 ms | 2.952 ms | 0.869 (0.818 to 0.941) | 1.475 |

Against the previous build (A the old Xo, B the new): ctr 0.58 (0.573
to 0.588).

## CBC encryption kernel (2026-11-11)

cbc_encrypt runs in a new arm64 kernel (cpu.aes_cbc_enc10/12/14, ssa
a64aescbc.xo: the round keys in v16 to v30 for the whole message, one
block a pass since each needs the last). Same host and recipe, B from
branch lang3-aesn with both changes:

| workload | Go wall | Xo wall | wall (95%) | before |
|---|---|---|---|---|
| block | 5.874 ms | 6.499 ms | 1.106 (1.077 to 1.694) | 1.074 |
| cbcenc | 9.727 ms | 5.272 ms | 0.542 (0.537 to 0.552) | 1.128 |
| cbcdec | 6.017 ms | 4.486 ms | 0.745 (0.735 to 0.752) | 0.735 |
| ctr | 3.174 ms | 2.741 ms | 0.863 (0.822 to 0.939) | 1.475 |

Against the previous build: cbcenc 0.478 (0.457 to 0.48). What is left
is `block` (a single encrypt_block per call, 1.1: the call and the Bytes
it returns), the next experiment there being the result written into a
Str made at its final size with one store pair.
