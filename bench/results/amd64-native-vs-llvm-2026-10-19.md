# Native amd64 against LLVM on amd64 hosts (decision 0270 checks, 2026-10-19)

The checks decision 0270 named before making the native backend the
default on darwin/amd64 and linux/amd64 hosts: the goldens with the
native amd64 backend, and its speed against LLVM at the build defaults
(`xo build`: LLVM at -O2 with ThinLTO, split.xo; native amd64 with no
flags). Xo lang 5 (xo-fa), main 1f4ce3cc (code as of 8ac3ca0b).

Result: correctness passes, performance does not. HTTP plain is 1.24 to
1.28 times LLVM's instructions a request and the TLS handshake 1.24, and
JSON 1.24 (geometric mean), all over the 10% bar. Decision 0270 stays
Proposed; Xo lang 4 takes the gap (lead, 2026-10-19).

## Goldens

| host | runner | backend | result |
|---|---|---|---|
| linux/amd64 (Docker Desktop, emulated) | `scripts/docker-goldens.sh <darwin xo> amd64 amd64 WORK` | amd64, no C toolchain | 199 of 199 |
| darwin/amd64 (Intel Mac, i7-9750H) | `scripts/intel-goldens.sh <darwin xo> HOST xo-ci/amd64-default-20261019 WORK amd64` | amd64 | 199 of 199 |

No failures, so no native gaps to list on the default path.

## HTTP server, instructions a request (linux/amd64, callgrind)

`ARCH=amd64 bench/perf/http/callgrind-docker.sh <darwin xo> OUT`
(MODE=full: N=100, REPS=2, both backends; valgrind 3.24.0 in Docker
Desktop's emulated linux/amd64, whose counts do not depend on load; the
Mac's load average was 37 to 91 from other sessions' gates). Minimum of
the two repeats:

| scenario | native amd64 | LLVM | native/LLVM |
|---|---:|---:|---:|
| plain | 31,215 | 25,123 | 1.242 |
| plain-newconn | 40,914 | 32,837 | 1.246 |
| tls | 59,497 | 58,177 | 1.023 |
| tls-newconn | 3,748,881 | 3,020,927 | 1.241 |
| h2 | 100,636 | 97,408 | 1.033 |

Per function (2N run minus N run over the requests, the Xo name for
both backends): one backend per OUT, since the ann files are named by
scenario only:

    ARCH=amd64 ANN=1 REPS=1 SCENARIOS="plain tls-newconn" bench/perf/http/callgrind-docker.sh <xo> OUT amd64
    (copy OUT/run/ann.* and req.* to DIR-amd64; the same with llvm to DIR-llvm)
    bench/perf/http/cgcompare.py --xo-names DIR-llvm DIR-amd64 plain tls-newconn

tls-newconn (+730k a request) is the field arithmetic: x25519 +381k
(fe64_mul +100k, fe64_sq +99k, fe64_sub 120k and fe64_add 49k called
where LLVM inlines them), nistec +172k (pt_add256 and pt_mul_g called,
mul +78k), mlkem +38k, the cabi bridge for obj_rs_append_bytes 25k, and
obj.rl_free_n's slow half called (35k). SHA-256 is even (LLVM's
sha256.compress 506k, native digest_tables.sha256_block 513k).

plain (+6.8k of 24.4k): obj +1.6k (rl_free_n_slow and rl_free called
where LLVM inlines), cabi +1.9k (obj_rl_copy_n through the bridge),
http and http1w helpers called (serve_conn, match_parts, head_request,
http1w.response, scan.find), map.put[Map[Str, Str]] 604, memmove +477;
alloc.alloc is the other way (native 1,198, LLVM 1,844).

## Compute benches, darwin/amd64 (Intel Mac)

Built on the arm64 Mac (`xo build --backend=amd64|llvm
--target=darwin/amd64`; LLVM cross builds darwin/amd64 with Apple's
clang), run on the Intel Mac (i7-9750H, 12 threads, 16 GB, macOS 26.5.2;
load average 0.53 at the start, 0.91 at the end). Each workload 5 times
as `/usr/bin/time -l BIN WORKLOAD`: the minimum instructions retired and
the minimum of the program's own microseconds (third output field;
rsa and ctx print none). The workloads are each bench's measure.sh
list. The tls and httpparse benches need their Go peers (serve/, load/)
and were not run this way; the HTTP and TLS handshake figures above
cover those paths. Checksums agree for every row.

Geometric means over the workloads (without `none`, the start alone):

| bench | workloads | instr native/LLVM | min | max | wall native/LLVM |
|---|---:|---:|---:|---:|---:|
| json | 11 | 1.237 | 1.101 | 1.716 | 1.389 |
| digest | 5 | 1.027 | 0.913 | 1.079 | 1.000 |
| rsa | 4 | 1.439 | 1.296 | 1.531 | - |
| text | 22 | 1.223 | 0.739 | 1.871 | 1.303 |
| portable | 10 | 1.531 | 1.158 | 2.292 | 2.522 |
| atomic | 8 | 1.179 | 0.965 | 1.797 | 1.168 |
| ctx | 5 | 1.049 | 0.843 | 1.181 | - |

portable's wall ratios run far past its instruction ratios (conv 2.10
instructions, 7.08 time; u64 2.29 and 4.78; shift 1.85 and 4.21; arith8
1.24 and 3.87): native loses more to stalls than to instruction count
there (a dependency through memory, a partial register write, or a slow
instruction; worth a look with the per-loop code).

Every workload:

| bench | workload | native instr | LLVM instr | instr ratio | native us | LLVM us | wall ratio |
|---|---|---:|---:|---:|---:|---:|---:|
| json | none | 40,094,474 | 37,158,684 | 1.079 | 102 | 107 | - |
| json | small | 382,139,707 | 323,936,484 | 1.180 | 31521 | 23612 | 1.33 |
| json | ints | 246,673,468 | 213,043,441 | 1.158 | 19557 | 15373 | 1.27 |
| json | floats | 413,209,389 | 375,235,664 | 1.101 | 39088 | 30011 | 1.30 |
| json | strings | 366,739,295 | 301,646,760 | 1.216 | 27154 | 19987 | 1.36 |
| json | maps | 370,850,825 | 318,568,342 | 1.164 | 30155 | 23592 | 1.28 |
| json | parse | 311,744,348 | 269,547,362 | 1.157 | 24068 | 18720 | 1.29 |
| json | invalid | 238,364,374 | 204,640,746 | 1.165 | 18737 | 14050 | 1.33 |
| json | enc_small | 167,456,072 | 97,595,958 | 1.716 | 12043 | 6034 | 2.00 |
| json | enc_strings | 210,408,121 | 166,162,991 | 1.266 | 14638 | 9417 | 1.55 |
| json | enc_floats | 165,105,321 | 110,955,824 | 1.488 | 13952 | 8870 | 1.57 |
| json | bytes | 436,133,386 | 391,366,678 | 1.114 | 34321 | 29806 | 1.15 |
| digest | none | 13,855,062 | 16,538,082 | 0.838 | 6 | 4 | - |
| digest | sha256_big | 759,277,765 | 728,030,077 | 1.043 | 48030 | 50486 | 0.95 |
| digest | sha256_small | 691,277,074 | 658,985,208 | 1.049 | 50535 | 51712 | 0.98 |
| digest | hmac | 967,872,972 | 910,887,885 | 1.063 | 65803 | 66640 | 0.99 |
| digest | pbkdf2 | 1,692,631,661 | 1,568,558,574 | 1.079 | 106062 | 107144 | 0.99 |
| digest | argon2 | 237,937,573 | 260,680,849 | 0.913 | 21235 | 19339 | 1.10 |
| rsa | sign2048 | 1,433,315,456 | 993,831,458 | 1.442 | - | - | - |
| rsa | verify2048 | 66,572,418 | 51,368,630 | 1.296 | - | - | - |
| rsa | sign4096 | 9,113,339,675 | 5,952,465,312 | 1.531 | - | - | - |
| rsa | verify4096 | 212,402,119 | 141,808,075 | 1.498 | - | - | - |
| text | none | 20,142,244 | 21,709,934 | 0.928 | 66 | 66 | - |
| text | parse_int | 48,984,811 | 47,479,719 | 1.032 | 3403 | 2789 | 1.22 |
| text | dur_fmt | 173,711,514 | 128,655,360 | 1.350 | 16527 | 11077 | 1.49 |
| text | dur_parse | 189,507,125 | 153,564,198 | 1.234 | 15322 | 11203 | 1.37 |
| text | time_fmt | 225,298,300 | 151,576,151 | 1.486 | 17724 | 15362 | 1.15 |
| text | time_parse | 144,157,736 | 109,072,598 | 1.322 | 9633 | 6173 | 1.56 |
| text | quote | 139,119,142 | 114,219,056 | 1.218 | 9457 | 6911 | 1.37 |
| text | split | 112,128,037 | 91,771,558 | 1.222 | 8570 | 6353 | 1.35 |
| text | lines | 178,668,927 | 139,937,122 | 1.277 | 14903 | 11051 | 1.35 |
| text | replace | 89,636,515 | 73,000,720 | 1.228 | 5055 | 3735 | 1.35 |
| text | trim | 186,435,334 | 150,077,825 | 1.242 | 13989 | 11258 | 1.24 |
| text | rune_count | 39,410,057 | 53,341,080 | 0.739 | 1684 | 2420 | 0.70 |
| text | utf8 | 143,556,055 | 147,665,134 | 0.972 | 9067 | 8683 | 1.04 |
| text | int_fmt | 86,843,268 | 68,348,681 | 1.271 | 5425 | 4000 | 1.36 |
| text | float_fmt | 163,607,951 | 110,633,459 | 1.479 | 14064 | 9784 | 1.44 |
| text | float_parse | 165,348,920 | 147,413,232 | 1.122 | 18069 | 13099 | 1.38 |
| text | base64 | 176,135,425 | 158,128,340 | 1.114 | 12832 | 10685 | 1.20 |
| text | hex | 121,463,632 | 101,631,541 | 1.195 | 7534 | 5547 | 1.36 |
| text | debug | 260,274,629 | 139,080,158 | 1.871 | 20952 | 9978 | 2.10 |
| text | zone_offset | 111,937,284 | 96,586,046 | 1.159 | 10226 | 8753 | 1.17 |
| text | zone_fmt | 1,150,274,828 | 775,946,592 | 1.482 | 114962 | 69159 | 1.66 |
| text | zone_civil | 297,941,952 | 220,714,095 | 1.350 | 26396 | 18951 | 1.39 |
| text | zone_load | 50,368,300 | 51,072,203 | 0.986 | 2826 | 2863 | 0.99 |
| portable | none | 13,480,278 | 16,844,670 | 0.800 | 8 | 7 | - |
| portable | arith32 | 119,595,119 | 87,017,971 | 1.374 | 30934 | 15998 | 1.93 |
| portable | arith8 | 105,631,889 | 85,013,699 | 1.243 | 30274 | 7828 | 3.87 |
| portable | u64 | 153,703,222 | 67,047,424 | 2.292 | 44491 | 9317 | 4.78 |
| portable | divmod | 381,465,672 | 223,030,227 | 1.710 | 137317 | 47438 | 2.89 |
| portable | shift | 201,628,828 | 109,016,771 | 1.850 | 28893 | 6857 | 4.21 |
| portable | conv | 308,179,955 | 146,819,712 | 2.099 | 65029 | 9185 | 7.08 |
| portable | str_hash | 493,056,190 | 425,859,870 | 1.158 | 55798 | 42434 | 1.31 |
| portable | unix | 130,270,472 | 111,508,394 | 1.168 | 8786 | 7027 | 1.25 |
| portable | checked | 333,625,166 | 198,914,355 | 1.677 | 22164 | 19914 | 1.11 |
| portable | float32 | 133,621,782 | 111,021,742 | 1.204 | 28518 | 15472 | 1.84 |
| atomic | none | 11,418,222 | 15,190,310 | 0.752 | 2 | 1 | - |
| atomic | get | 25,452,517 | 24,250,696 | 1.050 | 1435 | 701 | 2.05 |
| atomic | set | 21,466,904 | 19,867,993 | 1.080 | 8442 | 8397 | 1.01 |
| atomic | swap | 49,471,737 | 47,268,972 | 1.047 | 12128 | 11912 | 1.02 |
| atomic | cas | 35,461,031 | 31,259,095 | 1.134 | 12301 | 11660 | 1.05 |
| atomic | add | 133,463,735 | 91,268,332 | 1.462 | 16816 | 14664 | 1.15 |
| atomic | max | 71,464,694 | 65,263,414 | 1.095 | 6437 | 6154 | 1.05 |
| atomic | flag | 48,792,083 | 50,585,923 | 0.965 | 16325 | 15545 | 1.05 |
| atomic | contended | 151,190,787 | 84,139,783 | 1.797 | 115235 | 92440 | 1.25 |
| ctx | none | 11,696,163 | 15,414,105 | 0.759 | - | - | - |
| ctx | defer | 1,293,438,120 | 1,197,253,749 | 1.080 | - | - | - |
| ctx | mutex | 853,506,413 | 775,224,821 | 1.101 | - | - | - |
| ctx | spawn | 83,837,601 | 70,994,355 | 1.181 | - | - | - |
| ctx | budget | 1,198,177,379 | 1,421,309,844 | 0.843 | - | - | - |
| ctx | await | 88,509,147 | 82,497,653 | 1.073 | - | - | - |

## Gaps, ranked

1. Field arithmetic code in x25519 and nistec (fe64_mul, fe64_sq, mul):
   most of the TLS handshake gap and RSA's 1.44.
2. Small hot functions not inlined: fe64_add/sub, pt_add256, the fast
   half of obj.rl_free_n, the http and http1w helpers.
3. Runtime calls through the cabi bridges (obj_rl_copy_n,
   obj_rs_append_bytes) where LLVM calls the helper directly.
4. JSON encoding (enc_small 1.72, enc_floats 1.49) and text formatting
   (debug 1.87, dur_fmt 1.35, time_fmt 1.49, float_fmt 1.48, zone_fmt
   1.48): string building.
5. portable's integer loops: instructions 1.5 times LLVM's, time 2.5
   times (stalls, above).

Xo lang 4's first step (2026-10-19, in a gate when this was written):
one mulq for a mul and umulh pair and vector copies of word runs; plain
1.278 to 1.259, tls-newconn 1.249 to 1.209 on main a083d59b.

## Closing the gap (2026-10-19, Xo lang 4)

linux/amd64 callgrind (bench/perf/http/callgrind-docker.sh, ARCH=amd64,
REPS=1, N=100), instructions a request, native against LLVM of the same
main; LLVM plain 23,922, tls-newconn 2,972,410 (main a083d59b):

| Step | plain | ratio | tls-newconn | ratio |
|---|---|---|---|---|
| main a083d59b | 30,566 | 1.278 | 3,713,594 | 1.249 |
| 1: x86_mul_pair, x86_copy_runs | 30,118 | 1.259 | 3,594,873 | 1.209 |
| 2: x86_fold_loads | 30,030 | 1.255 | 3,553,572 | 1.196 |
| 3: loads with several reads folded, subtract with borrow chains | 29,283 | 1.224 | 3,453,221 | 1.162 |

1. compiler/arm64 x86.xo x86_mul_pair: a block's low (`mul`) and high
   (`umulh`) halves of one product make one mulq (x25519 fe64_mul 224 to
   200 instructions); x86emit.xo x86_copy_runs: runs of word copies
   (pushq/popq, or through r11) move 16 bytes a movdqu through xmm15
   (20,409 movdqu in the HTTP server).
2. x86.xo x86_fold_loads: a single use load (`movq OFF(base)`) becomes
   the memory source of its reader (add, adc, sub, sbb, logic, imul,
   cmp, test, mov, mulq); crypto_prims' static instructions 427,107 to
   410,150 (-4.0%), fe64_add 38 to 26 with no frame.

3. x86.xo x86_fold_loads folds a load read several times when every
   read can take the memory operand (fe64_mul's limbs, each a mulq's
   rax); std's field subtractions (scripts/gen/x25519fe64 sbb_chain,
   x25519 fe64_sub; nistec sub4, sub6, sub_m6) are x - y for the first
   word with 1 ^ its borrow carried into an adc chain of ~y (subs and
   sbcs on arm64, sub and sbb on x86, where bits.Sub64's formula took 7
   instructions a word on amd64), the add backs adc chains too; arm64's
   borrow_flag_peep keeps subs's carry flag into the first sbcs. The
   first word is not x + ~y + 1: LLVM joined its two carries with a
   conditional select (TestConstantTimeLLVM caught it). The Mac sweep:
   tls/p384 0.963, the rest within spread.

Correctness at each step: the amd64 goldens in Docker (199/199, no C
toolchain), progen arith 1..250 on amd64 (the six divergences seen there
were on main too: an overflow message's operand, fixed separately,
gotchas/amd64-fault-operand-spilled-slot.md).

### The rest of the gap by cause (after step 2)

bench/perf/http/cgmix.py over callgrind --dump-instr=yes runs (native
main b48b8156, LLVM main), instructions a request by bucket, native
minus LLVM:

| Bucket | plain native | plain LLVM | diff | tls-newconn native | LLVM | diff |
|---|---|---|---|---|---|---|
| load | 4,353 | 2,178 | +2,175 | 436,364 | 197,935 | +238,429 |
| move (register) | 4,355 | 2,838 | +1,517 | 764,257 | 624,706 | +139,551 |
| RC/drop helpers | 2,485 | 1,290 | +1,194 | 37,279 | 7,843 | +29,436 |
| call/frame | 4,216 | 3,420 | +796 | 234,660 | 168,732 | +65,928 |
| spill/stack | 2,465 | 2,004 | +461 | 473,848 | 254,947 | +218,901 |
| branch/cmp | 7,776 | 7,348 | +428 | 295,794 | 281,363 | +14,431 |
| overflow check (jo) | 357 | 302 | +55 | 12,912 | 7,338 | +5,573 |
| ALU with memory | 69 | 314 | -245 | 188,634 | 126,394 | +62,240 |
| ALU | 2,039 | 2,734 | -695 | 1,012,356 | 1,216,465 | -204,109 |

Loads and spills are about 60% of both gaps (the leaders: http1
parse_head and head_of, alloc.free_mt, map.put on plain; x25519
fe64_mul and fe64_sq, sha256_block, keccak, nistec mul and mul4p on
tls-newconn), then register moves (two operand ties, parallel moves),
then the RC helpers LLVM inlines and call overhead.

### Compute benches by cause (2026-10-19 night, Xo lang 4)

linux/amd64 callgrind (Docker, valgrind, one run each, XO_INLINE_STRICT=1),
instructions of the whole run, native against LLVM on main 1ff0e0bd:

| Bench | native | LLVM | ratio | where the difference is |
|---|---|---|---|---|
| portable u64 | 141,790,308 | 53,419,080 | 2.65 | UInt64 compares as xors and a signed compare |
| portable conv | 296,272,375 | 139,201,100 | 2.13 | divisions by narrow constants as idivq |
| portable shift | 189,730,792 | 91,388,434 | 2.08 | shift checks as two compares; a narrow constant amount in a register |
| portable checked | 317,731,751 | 185,377,215 | 1.71 | checked_mul's dispatch on its constant op code, inlined and not folded |
| portable divmod | 369,577,541 | 245,383,087 | 1.51 | idivq for narrow constant divisors |
| json enc_small | 147,401,300 | 83,740,240 | 1.76 | memmove (34M against 10M: byte loops for short copies), obj.rs_append calls |
| json small | 370,194,229 | 301,663,729 | 1.23 | |
| json strings | 361,281,256 | 290,591,987 | 1.24 | |
| rsa sign2048 | 1,413,945,083 | 983,777,184 | 1.44 | bigmod.mac_row (79%): eight words loaded up front, src, k and three words spilled |
| rsa verify2048 | 54,485,101 | 36,104,253 | 1.51 | the same loop |

Steps:

1. UInt64 compares unflipped to unsigned conditions, a shift's range
   check one unsigned compare, narrow constant shift amounts as
   immediates (isel unflip, show.xo, state.xo const_word): portable u64
   141.8M to 117.8M, shift 189.7M to 153.7M; arm64 sweep shift 0.808,
   u64 0.911, conv 0.978, 0 slower.
2. Next (gating): x86 divisions by narrow constants through the
   multiply high (conv 296M to 262M, u64 to 87.8M, divmod to 347.6M
   when measured with a general look-through; arm64 keeps sdiv, which is
   fewer instructions there), testq dropped after an op that set ZF,
   memmove and memcpy of up to 16 bytes as two overlapping loads and
   stores, mac_row loading each word where it is used (amd64 loop 66 to
   55 instructions a 4 word round, arm64 unchanged).

The HTTP numbers above move by about 1.5% a request from one run to the
next with the scheduler's spinning (pool.steal_round, find_work,
park_wait): one comparison read +330 instructions on plain, of which the
scheduler functions were +1,616 and the rest +78
(gotchas/scheduler-instruction-counts-spin.md). Compare per function
without them.

### 2026-10-20 (Xo lang 4): the rest of the steps, and two that did not pay

Pushed, linux/amd64 callgrind (each the same xo with the change off and
on, or main before and after):

| Change | Commit | Effect |
|---|---|---|
| x86 divisions by narrow constants (multiply high), testq after a ZF op, memmove/memcpy of up to 16 bytes | 0112f85f | u64 117.8M to 87.8M, conv 296.3M to 252.3M, divmod 369.6M to 323.6M, shift 153.7M to 133.7M; json enc_small 147.4M to 129.0M; HTTP plain 28,233 to 27,998 |
| branches on constants folded after the inliner (not with landings) | 33864ce5 | arm64 portable/checked 0.692 |
| x86_sink_loads | a396740a | rsa sign2048 -5.9%, verify2048 -6.9% |
| x86_const_imm (32 bit constants as immediates; lea adds, msub's product, branch compares) | de11c4e1 | unix -8.4%, arith8 -7.4%, conv -5.6%, float32 -5.6%, checked -4.4%, divmod -2.5% |

native/LLVM on main de11c4e1: HTTP plain 27,726 / 22,731 = 1.220,
tls-newconn 3,399,481 / 2,943,744 = 1.155. Compute benches (LLVM from
the 1ff0e0bd runs): u64 1.60, conv 1.71, shift 1.44, checked 1.17,
divmod 1.29, json enc_small 1.54, rsa sign2048 1.35.

Did not pay:

- bigmod.mac_row loading each word where it is used (the std source):
  amd64 rsa sign2048 1.44 to 1.27 of LLVM, but arm64 rsa and the TLS
  RSA and P-521 workloads 2 to 8.5% slower (dst's loads no longer pairs:
  1 ldp against 7). x86_sink_loads does part of it for x86 only.
- Str/List release's fast path (count above one, not shared: decrement
  in line) at every site: amd64 HTTP plain -0.5% (within the scheduler's
  spread), arm64 sweep 30 workloads 0.6 to 13% slower (digest/argon2
  1.134, portable/arith32 1.054): most releases are of a last reference,
  so the inline test runs and the helper is called anyway. A loops only
  variant was also slower on text benches. The helper's call stays.

Per function (plain), the native only costs are mostly functions LLVM
inlines: http1 read_one, the drop helper, alloc.free_mt, obj.rl_free,
map.put, obj_rl_copy_n, match_parts, scan.find, alloc_pop
(knowledgebase: the inliner list Xo Lang 2 holds).
