# SIMD survey of std's byte paths (2026-11-06)

Owner request (2026-11-06, through the lead): survey std's byte heavy
paths for SIMD, rank the candidates by time against Go, how often
programs hit them and the expected gain, implement the top ones, then
write a Proposed ADR for auto-vectorization in the native backend.

Program: bench/perf/simdscan (its Go twin twin.go.txt): 64 buffers of
4 KiB of mixed text (64 B for index_byte_64), each workload run over
them `passes` times. Host: linux/arm64 offload host 1, Go 1.26.1, xo
of main 7dbac5e08 (the simd worktree), `xo bench --ab --no-guard
--rounds 5 GO XO`; B/A is Xo over Go, below 1 is Xo faster. Every
workload's checksum equals Go's. Raw output:
~/xo-lead/notes/lang1/simd2-host.txt (not committed; the table is).

## Survey (before any change)

| Workload | std path | Xo ms | Go ms | Xo/Go | Profile (XO_PROF=cpu) |
|---|---|---:|---:|---:|---|
| index_byte | Bytes.find of a byte: llrt/scan.xo find_byte, 16 B a step (rt.bytes_eq16) | 883.9 | 323.6 | 2.73 | find_byte 56%, eq16_1 43% |
| utf8_ascii | Bytes.to_str: utf8_valid's ASCII span, 16 B a step (rt.bytes_high16) | 1141 | 433.2 | 2.63 | span 84%, high16_1 15% |
| equal | Bytes ==: obj.str_eq, then memcmp | 907.7 | 395.0 | 2.30 | libcstr.memcmp 99% (8 B a step) |
| compare | Str <: str_cmp, then memcmp | 731.4 | 394.0 | 1.86 | libcstr.memcmp 99% |
| index_byte_64 | find_byte on 64 B buffers | 550.8 | 359.5 | 1.53 | |
| sha256 | crypto sha256 (SHA2 instructions) | 498.2 | 399.5 | 1.25 | block256 91% |
| base64_dec | base64 decode | 908.9 | 834.8 | 1.09 (CPU 0.94) | |
| crc32 | crc32 IEEE (CRC32 instructions) | 930.6 | 884.5 | 1.05 | |
| utf8_mixed | utf8_valid of multibyte text | 995.2 | 1072 | 0.93 | |
| base64_enc | base64 encode | 532.9 | 906.7 | 0.59 | |
| hex | Bytes.to_hex | 242.4 | 921.2 | 0.26 | |

On linux the native build has no C library (XO_NOLIBC), so `memcmp`
is std/native/llrt/libcstr.xo's, which compared 8 bytes a step and
then byte by byte: that, not a missing intrinsic, is the whole of the
equal and compare gaps. On darwin the same calls go to libSystem's
memcmp.

## Ranking

Rank by gap times reach (how often a backend program runs the path):

1. memcmp (equal, compare): every Str and Bytes `==` over 16 bytes,
   every `<`, Map keys that collide, sort of strings. Gap 1.9 to 2.3x.
2. find_byte (index_byte): Str.find/split/lines of one byte, the HTTP
   head parser's line search, CSV and JSON scanning. Gap 2.7x.
3. utf8_valid's ASCII span (utf8_ascii): every Bytes.to_str of a body,
   every read of text. Gap 2.6x.
4. find_byte on short inputs (index_byte_64): header lines and fields.
   Gap 1.5x.
5. sha256: TLS and signing; 1.25x on the same SHA2 instructions, so
   scheduling, not vectorization.
6. crc32, base64_dec: within 10%; not worth an intrinsic.

## Step 1: std loops (no new intrinsic)

std/native/llrt/libcstr.xo memcmp compares four words (32 bytes) a
step with one branch, then a word a step, and finds the first
differing byte of a word by the lowest set bit of the two words' xor
(little endian), the last partial word read as the word ending at n.
std/native/llrt/scan.xo find_byte and span (the ASCII run of
utf8_valid) test two 16 byte masks a step with one branch, and their
loops step with wrapping arithmetic (each `i += 16` and `n - i` was an
overflow check and a branch, scan.xo's disassembly on host 1).

Measured with one xo (main's) building simdscan against main's std (A)
and the new std (B), `xo bench --ab --no-guard --rounds 5`, host 1:

| Workload | B/A | B/Go (was) |
|---|---:|---:|
| equal | 0.730 | 1.646 (2.298) |
| compare | 0.737 | 1.336 (1.856) |
| index_byte | 0.737 | 1.913 (2.731) |
| utf8_ascii | 0.598 | 1.264 (2.634) |
| index_byte_64 | 0.872 | 1.244 (1.532) |
| utf8_mixed | 1.033 (within noise) | 0.855 (0.928) |

Raw output: ~/xo-lead/notes/lang1/simd3-host.txt.
