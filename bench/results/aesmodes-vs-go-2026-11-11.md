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
