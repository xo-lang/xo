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

## Step 1b: ASCII case folding and html escape (std only)

std/native/llrt/scan.xo fold_eq_words compares 8 byte words while both
are ASCII and equal, or equal with their capitals lowered (the SWAR
range test `(w + (0x80-lo)*ONES) & ~(w + (0x7f-hi)*ONES) & HIGHS`),
for `equal_fold` (Str and Bytes), `eq_lower` and http1.xo's header
name compare; case_run finds the first byte `to_lower`/`to_upper`
changes a word at a time. html.xo escape_string counts each word's
specials by mask (popcount) and jumps to a word's first special (ctz).

Measured as step 1 (one xo, new std against main's, `--rounds 5`,
host 1), simdscan's new workloads (fold: 64 buffers of 4 KiB against
their upper case; fold_32: 32 byte prefixes; lower: to_lower of upper
case text):

| Workload | F/A | F/Go |
|---|---:|---:|
| fold | 0.185 | 0.165 |
| fold_32 | 0.212 | 0.245 |
| lower | within noise (1.024) | 0.136 |
| html escape (bench/perf/html) | 0.943 | within noise (0.721) |
| bytestransform upper | 1.041 | |
| bytestransform lower | 0.927 | |

bytestransform's fold row stayed within noise: its inputs mostly differ
in length, so the loop rarely runs. Its upper row is the cost of one
word read before the first byte to change in its short lowercase texts.
A first html version that tested a word at the top of the byte loop
measured 2.444 of main's (gotchas/swar-skip-in-byte-loop.md). Raw
output: ~/xo-lead/notes/lang1/simdfold-host.txt and simdfold2-host.txt.

## Not taken: websocket's mask 32 bytes a step

std/native/websocket.xo mask_to already XORs a word a step; four words
a step with wrapping index arithmetic measured within noise on
bench/perf/websocket (host 1, `--rounds 7`, against main's std): big
(16 KB messages) 1.078 (0.951 to 1.253), small 1.004 (0.996 to 1.033).
The loopback round trip dominates both. Raw output:
~/xo-lead/notes/lang1/wsmask-host.txt.
