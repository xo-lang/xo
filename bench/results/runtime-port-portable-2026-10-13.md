# Runtime port: the zero-C audit's portable group (2026-10-13)

Decision 0195's "(0) portable now" group of
`bench/results/zero-c-audit-2026-10-13.md` (1,799 lines of std/llrt that
need no new prerequisite), moved under decision 0193's mechanism (an
`std/native/llrt/<name>.xo` module, its C path `<name>_c.xo` selected
by `XO_LLRT_C=<name>`, the C under `#ifdef XO_<NAME>_C` with an IOFlags
entry) and measured against decision 0180's bar (each workload within 5
percent of the C path's instructions on arm64 and LLVM). Machine: the
shared 12 core Apple M series Mac (darwin arm64), several agents running;
every run through the heavy lock, one at a time. Figures are
instructions retired (`/usr/bin/time -l`), the minimum of 5 runs, less
the setup (the `none` workload or test); native builds used the Go
driver (`XO_SELF_DRIVER=0`; the self hosted SSA builder, arm64 and LLVM
emitters, and assembler mirror every change). Not in scope here and not
touched: the "(0) held by C callers" group (exporting Xo to C is owner
decision C) and port6's xo_http.c, xo_h2.c, xo_fs.c path policy, and
xo_proc.c argv.

## What moved

| Piece | Was (C) | Now | C path |
|---|---|---|---|
| Sized integer arithmetic, shifts, negation, conversions, `Rune(n)`, `Int32.try` and `Rune.try` | xo_bytes.c `xo_int_op`, `xo_int_conv`, `xo_rune_of`, `xo_int_in_range`, `xo_rune_valid` (a C call per operation) | lowered inline by both SSA builders (internal/ssa build_int.go `sizedOp`, selfhost/ssa show.xo `sized_op`): the word operation and its range check; the faults' text in std/native/llrt/ints.xo (`op_fault`, `conv_fault`), UInt64 `/` and `%` there (`u64_div`, `u64_rem`) | `XO_LLRT_C=ints`, `-DXO_INTS_C` |
| `Time.unix`, `unix_millis` | xo_rt.c `xo_floor_div` | ints.xo `floor_div` | ints |
| The hash of a Str Map key | xo_rt.c `xo_str_hash` | ints.xo `str_hash` (FNV-1a eight bytes a word) | ints (and std/json's C path keeps it) |
| Int's `checked_` and `saturating_` methods, `pow` | xo_bytes.c `xo_int_fits` | ints.xo `fits`, `apply` | ints |
| The C versions of the intrinsics | xo_bytes.c `xo_int_wrap`, `xo_u64_mul_hi`, `xo_float_bits`, `xo_float_from_bits`, `xo_bytes_word`, `xo_int_fits` | compiled only when the generated code calls one (`-DXO_INTS_C` from IOFlags); nothing does by default | ints |
| UInt64 to Float, to Float32, Float to Float32 | xo_bytes.c `xo_u64_to_float`, `xo_f32_round`, `xo_i64_to_f32` (a C call per Float32 operation) | intrinsics lowered inline by both arm64 emitters (`ucvtf`, `fcvt` through the single precision register, `scvtf`) and both LLVM emitters (`uitofp`, `fptrunc`/`fpext`, `sitofp`) | the C only when called |
| MemFs.fail_next's pattern match (path.Match) | xo_testing.c `xo_test_glob` | std/native/llrt/glob.xo | `XO_LLRT_C=glob`, `-DXO_GLOB_C` |
| SHA-256, HMAC-SHA256, PBKDF2-HMAC-SHA256, unpadded base64, verify_password's parsing | xo_crypto.c | std/native/llrt/digest.xo, SHA-256's compression generated unrolled (digest_tables.xo, TestDigestTables) | `XO_LLRT_C=digest`, `-DXO_DIGEST_C` |
| The scalar values of property tests and testing.gen (genInt, sized integers, genFloat, genStr, Rune, Duration, Time, Size) | xo_testing.c `xo_gen_int` and friends | std/native/llrt/gen.xo over the C generator's draws (`xo_gen_draw`, `xo_gen_depth`) | `XO_LLRT_C=gen`, `-DXO_GEN_C` (`xo_gen_int` renamed `xo_gen_any_int` so IOFlags tells it from `xo_gen_intn`) |
| `xo_str_quote` | xo_rt.c, 44 lines | deleted: nothing called it | |

New intrinsics (internal, `use native/rt` modules only, no language
change): `rt.int_wrap` ops 5 (rotate right), 6 (the low 32 bits rotated
right, zero extended), and 7 (bytes reversed), lowered to `ror`, `ror w`,
and `rev` on arm64 and to `fshr` and `bswap` on LLVM, in WAT for the
wasm runtime and in C for any other call; `rt.u64_to_float`,
`rt.f32_round`, `rt.i64_to_f32` now inline as above. The self hosted
assembler gained `rev`, `fcvt` (single and double precision), and
`scvtf` to single precision (selfhost/macho/macho_test.xo checks the
encodings against the system assembler's).

## Line counts

std/llrt lines out of default builds: **1,040 of the group's 1,799**:
996 under the new guards (xo_bytes.c 263 under `XO_INTS_C` and 12 under
`XO_NUM_C || XO_DIGEST_C`, xo_rt.c 8 and 13, xo_testing.c 198 under
`XO_GLOB_C` and 97 under `XO_GEN_C`, xo_crypto.c 405 under
`XO_DIGEST_C`) and 44 deleted (`xo_str_quote`). They stay compiled for
the C paths and for the C test harnesses that include the runtime
(std/llrt/testdata/crypto_diff.c defines `XO_DIGEST_C`).

What stays C, 759 lines, and why:

- **BLAKE2b and Argon2id**, xo_crypto.c (about 368 lines): an Xo version
  (commit b54614f2: BLAKE2b's compression and Argon2's G written out
  unrolled, generated) missed the bar, so it was taken out (commit
  b1160720): verify of an Argon2id hash retired **2.071x the C's
  instructions on arm64 and 1.294x on LLVM** (table below). The cost is
  in the arm64 code shape: each List[Int] element access is an address
  add and a load (C uses one load with an offset), fBlaMka's
  `2 * lo(a) * lo(b)` is `and`, `and`, `mul`, `lsl` (C: one `umull` and a
  shifted add), and the 1 KiB blocks live in a List[Int] whose stores go
  through the list. verify_password's parsing is Xo and calls the C
  Argon2id.
- **SHA-512**, xo_crypto.c (114): held by its C caller, Ed25519 (group
  (5), constant time); it moves with Ed25519.
- **The generator's state**, xo_testing.c (about 227): the PCG state, the
  record of choices, the replay list parsed from `XO_PROP_REPLAY` and the
  seed from `XO_PROP_SEED`, the depth: a C struct behind an Int handle
  (calloc, realloc, getenv: groups (2) and (3) rather than (0)); and
  `tw_rand_u64` (20), the test world's Rand, held by xo_crypto.c's
  `crypto.random_bytes` C. The value policy moved (gen.xo).
- **File structure** (51): xo_std.c's header, blank lines, and includes
  (47) and xo_http.c's includes (4); they go when the files do.

## Final: bench/perf/portable (ints.xo and the Float32 intrinsics)

`bench/perf/portable/measure.sh`, "C" is the same compiler with
`XO_LLRT_C=ints`. Checksums are identical on both backends, both paths,
and equal the Go backend's. 2,000 rounds over 1,000 values each.

| Workload | arm64 Xo | arm64 C | less setup | LLVM Xo | LLVM C | less setup |
|---|---|---|---|---|---|---|
| `arith32` | 95,282,283 | 622,407,256 | 0.128 | 61,095,097 | 590,241,201 | 0.076 |
| `arith8` | 64,410,377 | 582,360,833 | 0.082 | 59,893,681 | 526,161,452 | 0.084 |
| `u64` | 120,168,836 | 654,525,008 | 0.161 | 45,946,111 | 588,351,016 | 0.050 |
| `divmod` | 190,170,569 | 2,286,358,005 | 0.076 | 159,936,065 | 2,186,220,354 | 0.066 |
| `shift` | 144,146,814 | 934,311,475 | 0.138 | 73,957,669 | 880,262,871 | 0.066 |
| `conv` | 238,690,733 | 1,071,977,577 | 0.209 | 103,963,299 | 929,165,962 | 0.095 |
| `str_hash` | 396,315,578 | 382,726,287 | 1.038 | 375,102,411 | 362,848,048 | 1.032 |
| `unix` | 82,720,771 | 130,262,750 | 0.578 | 74,572,939 | 88,120,308 | 0.797 |
| `checked` | 242,179,983 | 316,303,954 | 0.752 | 139,917,356 | 258,177,331 | 0.507 |

Setup (`none`): arm64 18,027,914 Xo, 18,321,341 C; LLVM 17,238,614 and
16,178,838. The bar holds for every workload on both backends;
`str_hash` (Map[Str, Int] lookups) is the closest, at 1.038 and 1.032.

Float32 (the intrinsics are not behind `XO_LLRT_C`, so "C" is the
compiler of commit cd80ffc7, before them, building the same program):

| Workload | arm64 now | arm64 cd80ffc7 | less setup | LLVM now | LLVM cd80ffc7 | less setup |
|---|---|---|---|---|---|---|
| `float32` | 136,108,280 | 174,143,139 | 0.756 | 95,867,639 | 159,950,892 | 0.556 |

## Final: bench/perf/digest (digest.xo)

`bench/perf/digest/measure.sh`, "C" with `XO_LLRT_C=digest`. The hashes
verified were made by the Go backend; checksums equal it.

| Workload | arm64 Xo | arm64 C | less setup | LLVM Xo | LLVM C | less setup |
|---|---|---|---|---|---|---|
| `sha256_big` (64 KiB, 200 times) | 584,096,425 | 707,367,271 | 0.820 | 475,910,861 | 695,874,523 | 0.675 |
| `sha256_small` (100,000 messages under 120 bytes) | 595,074,820 | 602,664,902 | 0.987 | 502,853,848 | 587,496,714 | 0.851 |
| `hmac` (50,000) | 825,138,944 | 835,486,663 | 0.987 | 680,540,143 | 819,878,953 | 0.826 |
| `pbkdf2` (verify, 20,000 iterations, 10 times) | 1,367,863,558 | 1,466,613,570 | 0.932 | 1,087,214,948 | 1,434,229,776 | 0.755 |
| `argon2` (verify, 4 MiB, 2 passes, 4 times; the C on both paths) | 201,971,514 | 201,937,183 | 1.000 | 199,396,708 | 199,365,504 | 1.000 |

Setup: arm64 22,035,533 Xo, 21,957,184 C; LLVM 19,195,460 and
19,234,579.

The Argon2id in Xo of commit b54614f2 (the same bench, that commit):

| Workload | arm64 Xo | arm64 C | less setup | LLVM Xo | LLVM C | less setup |
|---|---|---|---|---|---|---|
| `argon2` | 394,694,379 | 201,939,207 | 2.071 | 252,251,652 | 199,372,428 | 1.294 |

## Final: bench/perf/glob and bench/perf/gen (test binaries)

Each runs one test of a test binary (`XO_TEST`), less the setup test.
bench/perf/glob: 40 pending fail_next patterns that never match (each
scans most of the path, stars backtracking) against 2,000 writes, so
80,000 matches; bench/perf/gen: 20,000 testing.gen values of every
scalar kind.

| Test | arm64 Xo | arm64 C | less setup | LLVM Xo | LLVM C | less setup |
|---|---|---|---|---|---|---|
| glob (setup 36,917,468 and 36,835,585; LLVM 29,487,103 and 29,275,298) | 153,661,876 | 152,515,746 | 1.009 | 127,907,776 | 140,554,111 | 0.884 |
| gen (setup 36,957,575 and 36,960,609; LLVM 34,723,606 and 34,684,380) | 101,797,134 | 105,485,587 | 0.946 | 94,698,512 | 102,197,695 | 0.888 |

## How it got there

- **Sized integers** first lowered as an llrt call per operation would
  have depended on the inliner folding the type's constants; the SSA
  builders emit the check inline instead (a narrow type's exact result
  fits the word, so `+ - *` are the Int operation and `r >> bits` or `r
  + 2^(bits-1) >> bits` tested against 0; UInt64 checks the carry, the
  borrow, `u64_mul_hi`, and the shift back), and only the cold fault
  calls Xo.
- **glob** first measured 1.317 (arm64) and 1.204 (LLVM): its match loop
  kept a `failed` flag, the class parsing, and the pattern's bounds live
  in one function (about 35 instructions per byte, `si` spilled). Literal
  runs without the flag, the class and the after-failure syntax check in
  their own functions, and ASCII decoded inline: 1.009 and 0.884. On the
  way, a second inlined copy of `match_chunk` under a `&&` loop condition
  made internal/arm64 read a register before writing it
  (gotchas/arm64-and-loop-condition-inlined-twice.md, worked around).
- **SHA-256** first followed the C (a 64 word List schedule, eight moves
  a round, masks on every add): `sha256_big` 2.0x on arm64 (wall clock).
  Generated unrolled (the schedule in 16 locals, the working variables
  renamed, the constants immediate, the rounds only rotating the low 32
  bits so nothing is masked until the state add): 0.820 and 0.675. HMAC
  then measured 1.086 on arm64 from building its key states as lists;
  one Bytes of both pads and the outer block from the inner state: 0.987.
- **Argon2id**: the generated G (128 word blocks unrolled, bounds checked
  once by a guard the arm64 backend's relational facts use) took arm64
  from 2.24 to 2.07; see "What stays C".

## Correctness

- TestIntsDiff (internal/llvm/ints_test.go): every operation of Int8 to
  UInt64 on every pair of each type's bounds and on random pairs, with
  variable and constant operands, compound assignment, every conversion
  between them and to Int, Rune, Float, and Float32, the `try` forms, and
  Int's checked_, saturating_, wrapping_ methods and pow; 129,619 output
  lines on arm64 and LLVM, Xo and C paths, against the Go backend.
  UInt64 shift amounts of 2^63 and above are left out: the Go backend
  shifts them (gotchas/uint64-shift-amount-above-2-63.md).
- TestDigestDiff (internal/llvm/digest_test.go): SHA-256 and HMAC of
  every length to 140 and random data to 10 KB, verify_password on the
  x/crypto Argon2id outputs and Go PBKDF2 hashes (keys of one to four
  blocks), each mutated; 1,501 lines against Go's crypto packages and
  std/rt's verify_password, both backends, both paths. TestCryptoDiff
  (the C path) passes; goldens native_crypto and native_ed25519 on both
  backends and paths.
- TestGlobDiff (cmd/xo/glob_test.go): 600 patterns and names (Go's
  path.Match table and random ones, half the names made from their
  pattern; 209 match), one test each, against Go's path.Match on the Go
  backend and both native backends, both paths.
- TestGenDiff (cmd/xo/gen_diff_test.go): testing.gen of every scalar
  kind and collections of them for 100 seeds (600 tests), reports
  identical to the Go backend's, both backends, both paths; the property
  tests of cmd/xo/testdata/nogc/prop likewise.
- Goldens native_sized_ints, sized_overflow, shl_overflow, rune_fault,
  show_table, maps, map_remove_bytes, native_map_hash_wrap, float32,
  unicode_table, native_gaps, bytes_for, tz_table on both backends.
- TestIOFlagsInts, TestIOFlagsDigestAndGlob, TestXo1IOFlagsSymbols,
  TestLLRTCPath, TestDigestTables; selfhost/macho's Xo tests (the new
  encodings).
- Not run here (the lead's merge and full tiers): TestSelfhostSSA and the
  other selfhost tiers, the wasm goldens.

## Next, ranked

1. arm64 code shape for List[Int] heavy code, then Argon2id again: fold
   `xs[i + c]` into the load's offset (`offsetBase` returns the base and
   `loadElem`/`storeElem` take the extra offset), lower `(a & M) * (b &
   M)` with 32 bit masks to `umull`, fuse `eor` with a shifted operand;
   each in selfhost/arm64 first, mirrored in internal/arm64; then
   remeasure bench/perf/digest `argon2` from commit b54614f2's digest.xo.
2. SHA-512 in Xo (the unrolled generator of digest_tables.xo with
   64 bit words and `int_wrap` op 5), called by the C Ed25519 through
   decision 0204's export (owner, 2026-10-13: C may call Xo runtime
   functions), which also frees `tw_rand_u64` for the generator's state.
3. The generator's state in Xo: a List[Int] owned by the generated test
   code, passed `var` through the `xo.gen[T]` functions, with
   `XO_PROP_REPLAY` and `XO_PROP_SEED` read through the env primitives;
   PCG and Lemire's bound are already expressible (`u64_mul_hi`).
4. `str_hash` margin: the tail as one shifted word when the key has 8
   bytes or more.
5. Delete the C paths of ints, glob, digest, and gen after a full tier,
   as decision 0195's plan does for the earlier steps.
