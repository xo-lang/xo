# amd64 ties: a loop's accumulator counted live past its phi copy

Status: Observed (2026-11-08)

## Symptom

A sum loop on amd64 (`for x in b { s += Int(x) }`, ADR 0574's
geo_byte_sum) added into the loaded byte's register and copied the sum
back every iteration:

    movzbl (%rcx,%r10), %r10d ... addq %r8, %r10; jo ...; movq %r10, %r8

Making a commutative add tie the operand its phi copy carries the
result into (the first attempt) changed nothing.

## Cause

x86_retarget's dead uses (x86_dead) counted a use as dying only at its
last read in the block and only when it was not live out of the block.
The accumulator is live out (the loop's exit block reads it) but the
block's own phi copy (K_PMOVE `v4 <- v25`) writes it before that, so
its value at the add is dead. x86_dead missed it, the add saw both
operands as "the tied one dies, the other lives", and tied the byte.

## Fix

x86_dead comes from a backward liveness scan of the block where a
K_PMOVE's definitions end their earlier values (other instructions do
not, which keeps it conservative for templates that read their
destination), and a commutative operation whose operands both die ties
the one a later copy in the block carries the result into (x86_pm).
Find this kind of miss with `XO_DUMP_FN=NAME XO_DUMP_DIR=DIR
XO_BACKEND_CACHE=off xo build --backend=amd64`: the MIR after regalloc
shows each use's register and the copies.

## Measured (2026-11-08): faster on Intel, mixed on Zen 5

Against its parent, with bench/perf/bytesum (a 63 byte sum, 20 million
calls), bench/perf/checksum, digest, json, strconv and httpstreamsrv:

Intel i7-9750H (darwin/amd64, 7 interleaved rounds, medians):

| bench | before | after | ratio |
|---|---|---|---|
| bytesum | 699.8 ms | 402.2 ms | 0.575 |
| checksum adler32 | 433.3 ms | 426.9 ms | 0.985 |
| checksum fnv64a | 409.5 ms | 406.9 ms | 0.994 |

AMD Zen 5 (the linux/amd64 offload host): `xo bench --ab` wall, and for
runs under 60 ms perf stat cycles (-r 30, twice each way):

| bench | ratio | how |
|---|---|---|
| bytesum | 1.000 (0.995 to 1.001) | --ab, 3 rounds |
| checksum fnv64a | 0.995 | --ab, 3 rounds |
| checksum adler32 | 1.024, 1.023 (1.015 to 1.039) | --ab, 3 rounds, twice |
| checksum adler32 | 1.045 cycles | perf stat |
| checksum crc32 | 1.003 cycles | perf stat |
| digest sha256_big | 1.0005 cycles | perf stat |
| json parse | 1.004 cycles | perf stat |
| strconv itoa (30000 passes) | 0.988 (0.937 to 1.008) | --ab, 7 rounds |
| httpstreamsrv | 1.006 (0.988 to 1.012) | --ab, 3 rounds |

adler32 on Zen 5: 3% fewer instructions, 4% more cycles (IPC 7.00 to
6.51), equal branch misses and front end stalls; llvm-mca (znver4)
predicts the new loop faster (10.3 against 10.7 cycles an iteration).
Zen 5 renames the copy away, so it cost nothing there, and the 8-byte
loop (16 `jo` and 2 other branches an iteration, about 9.4 cycles) is
likely bound by branch throughput, so removing the copy only moved which
instructions compete for those ports (not proven). The Skylake class
Intel core paid for the copy on the accumulator's chain.

Aligning loop heads to 16 bytes (a pass x86_loop_align, tried and
dropped) did not help: adler32 1.037 with it, 0.979 alone, for +2.0%
__text on httpstreamsrv and +2.35% on xo (linux/amd64). Removing the
overflow tests themselves (decision 0577) is the fix for both cores.
