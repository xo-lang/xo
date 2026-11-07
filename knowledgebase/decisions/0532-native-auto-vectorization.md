# 0532: Auto-vectorization in the native backend

Status: Accepted (owner, 2026-11-07: "Nah keep going with your
strategy", on why there is no general vectorizer). Proposed by Xo Lang 1,
2026-11-06 (the owner's SIMD request of 2026-11-06, part 2). The order
stands: 1. intrinsics for search shapes, 2. idiom recognition into std
kernels, 3. a loop vectorizer only if step 2's measurements show the
rest matters, each step gated by a measurement before code.

## Context

The owner asked (2026-11-06, through the lead) for a survey of std's
byte heavy paths with the top ones made SIMD intrinsics (part 1,
bench/results/simd-survey-2026-11-06.md), then this ADR: what
automatic vectorization in the native backends would take and give.

What exists:

- Hand vectorized std paths through intrinsics (decision 0201's
  rt.bytes_eq16 and rt.bytes_high16, 0373's rt.case_fold16, the 32 byte
  rt.bytes_any_eq32, rt.bytes_any_high32 and rt.mem_ne32 of part 1's
  step 2 (code generation first, std use after a seed), the
  crypto kernels of compiler/ssa a64*.xo and x86*.xo): about 337
  sites (the survey below).
- Decision 0323: a two lane Float SLP pass on arm64 (slp_f64,
  slp_carry), with a wide vreg class in the register allocator (caller
  saved V registers only, 16 byte spill slots, `$q`/`$v` template
  names). Straight line and loop carried pairs; no loop vectorizer; not
  on x86_64.
- Decision 0366 (Rejected, 2026-10-30): a two lane Int widening pass for
  SWAR word loops measured 1.71 times the native loop on a hand written
  harness, under its plan's 2 times bar; reopening needs a measurement
  that clears it, such as four lanes.
- compiler/ssa unroll.xo: full unrolling of small counted loops; the
  arm64 MIR has LSR and loop invariant hoisting.
- LLVM backend: clang -O2 runs the loop and SLP vectorizers on the IR.

### Loop shapes (std and the goldens)

A read-only survey of std/native (about 4,486 non-test loops, the hot
areas read in full) and the goldens (internal/llvm 599, internal/gen
155, internal/arm64 run 27), 2026-11-06, classed as A elementwise map
or copy, B reduction, C search or early exit, D carried dependence, E
gather or table lookup, for loops not already hand vectorized:

| Class | std/native | goldens |
|---|---:|---:|
| A map or copy | 25 | 2 |
| B reduction | 9 | about 15 |
| C search, early exit | 17 | 0 |
| D carried dependence | 12 | about 3 |
| E table lookup | 6 | about 12 |

- Already hand vectorized: about 337 sites (17 lane intrinsic calls in
  scan.xo, obj.xo and libcstr.xo; 247 SWAR rt.bytes_word calls in 32
  files; about 73 crypto and checksum kernels).
- The hottest candidates for a backend service are search shapes (C):
  ASCII case insensitive compares (llrt/unicode.xo eq_lower,
  equal_fold_bytes, case_done; llrt/http1.xo equal_fold), the header
  token check (http1.xo `cls(ch) & TCHAR`), JSON unquote's scan for `\`
  or a byte at or above 0x80, URL escaping's prefix scans, metrics
  histogram bucket search. Class A's dense work is jpeg (dequantize,
  clamp, IDCT, color conversion), png's up unfilter, websocket's mask
  (SWAR, could go 16 or 32 bytes); class B html.xo escape sizing (`extra
  += ESC_EXTRA[c]`, a gather) and flate's bit_length.
- The hottest loop no vectorizer helps: llrt/ints.xo str_hash (FNV-1a,
  every `Map[Str]` lookup), each byte's multiply waiting on the last.
- In the body: about 90 percent of std candidates index Bytes or a List
  (bounds checks; decision 0184 removes some for `0..len` loops), about
  58 percent carry overflow checked arithmetic beyond the counter; the
  goldens' Int sums all use checked `+=`, and arm64/run fault_add_loop,
  fault_sub_loop, fault_neg_loop and lsr_fault_neg require the trap at
  the exact iteration. llrt/ct.xo and bcrypt.xo go through
  rt.ct_barrier so constant time compares are never vectorized or made
  early exits; any pass must leave them alone.

### Clang's ceiling

What vectorization buys LLVM on Xo's own IR: each bench/perf program
built with --backend=llvm twice, clang as is and with
`-fno-vectorize -fno-slp-vectorize` (XO_CLANG wrapper), and natively;
linux/arm64 host 1, `xo bench --ab --no-guard --rounds 3`:

Columns: LLVM without its vectorizers over LLVM with them (above 1
means the vectorizers help), and native over LLVM with them; "noise"
is `--ab`'s within noise verdict. xo of lang1-simd2 (9b2e88cc2, step
1's std: memcmp, find_byte and span are already the 32 byte loops),
clang 23.1.3.

| Program | Workload | LLVM no-vec / vec | native / LLVM vec |
|---|---|---:|---:|
| simdscan | equal | 1.500 | 1.395 |
| simdscan | index_byte | 0.948 | 1.370 |
| simdscan | utf8_ascii | 1.012 | 1.054 |
| simdscan | hex | 0.988 | 1.201 |
| simdscan | base64_enc | 0.993 | 1.086 |
| simdscan | base64_dec | noise | 1.044 |
| simdscan | crc32 | noise | noise |
| checksum | adler32 | noise | 1.013 |
| checksum | fnv64a, crc64 | noise | noise |
| compress | deflate | noise | 0.883 |
| compress | inflate | 1.017 | 0.908 |
| hash | fnv128a | noise | 0.607 |
| csv | parse | 0.993 | 1.191 |
| bytestransform | lower | noise | 1.051 |
| bytestransform | valid, index | noise | noise |
| bytessearch | find_any | noise | 1.429 |
| bytessearch | index | noise | noise |
| jpeg | decode | 0.944 | 1.117 |

- One row gains from clang's vectorizers: equal (1.5 times), where
  memcmp's loop of four word xors becomes 16 byte compares. That is the
  loop step 2b's rt.mem_ne32 writes by hand.
- Everywhere else the vectorizers are within 5 percent, and on two rows
  the vectorized build is slower (index_byte 5 percent, jpeg 6
  percent). The search rows gain nothing: the hot loops are already
  lane intrinsics, and the rest are early exit loops clang leaves
  scalar.
- Native's remaining gaps to LLVM (find_any 1.43, index_byte 1.37,
  equal 1.40, hex 1.20, csv 1.19, jpeg 1.12) are scalar code quality and
  the intrinsics' loop shape, not missing vectorization, except equal.
- Code size: the vectorized and unvectorized LLVM binaries differ by
  under 0.1 percent (jpeg 400 bytes larger, simdscan 976 bytes smaller).

Raw output: ~/xo-lead/notes/lang1/vecceil-host.txt (not committed; the
table is).

## Decision

Do not build a general loop vectorizer now. Build in this order, each
step gated by a measurement before code:

1. Keep the intrinsic route for search shapes (class C): an early exit
   loop needs a vector compare, a mask move and a scalar locate, which a
   pattern in std written once (scan.xo) serves for every caller; a
   vectorizer would need early exit support, which LLVM's own loop
   vectorizer has only in recent versions and for limited shapes (the
   ceiling run's search rows: clang 23 gains nothing on them).
   Part 1 did this for find_byte, span and memcmp; the survey's next
   ones are the ASCII case insensitive compare (eq_lower,
   equal_fold_bytes, http1 equal_fold: a 16 byte fold of both sides,
   0373's lane, then a compare), case_done, and the 8 byte SWAR scans
   that could go 16 or 32 (span class 1, ctl_at, mem_byte, count_cont,
   jsontext's string scan, the websocket mask). Jsontext's str_end and
   append_body already jump to the first stop of an 8 byte word by
   mask; a wider form needs a class intrinsic (any byte equal to one of
   two, or below 32, or from 0x80, in 16 or 32 bytes), one more
   code generation step and seed.
2. Idiom recognition for class A and B loops on Bytes (a byte map with a
   fixed operation set: xor or add of two ranges, a constant mask, a
   compare into 0/1; a count of a byte class; a sum of bytes): an SSA
   pass that recognizes the loop shape after inlining and unrolling and
   replaces it with a call of a vector kernel written in std (as
   memcpy idioms are), so both encoders get it through the kernel's
   intrinsics and the compiler gains no vector types. Measure first:
   a hand written kernel against native's loop on the top
   candidates (websocket's mask xor, png's up unfilter add, a count of
   UTF-8 continuation bytes, html escape's size of the specials), with
   decision 0366's 2 times bar on the loop alone.
3. A loop vectorizer proper (class A and B over Int and Float lists)
   only if step 2's measurements and the ceiling show the remaining
   loops matter. It would need:
   - Dependence analysis: Xo's value semantics help (a List or Bytes
     parameter is unique or shared, never aliased by a pointer; decision
     0322 proves uniqueness), so a loop that reads xs and writes ys has
     no cross iteration dependence unless ys is xs; index expressions
     must be affine in the induction variable.
   - The checks in the body: Int arithmetic traps on overflow and index
     reads are bounds checked. A vector body needs both hoisted: the
     bounds proven for the whole trip range before the loop (one check
     of the last index), and overflow either proven impossible (range
     analysis) or the body written with wrapping operations; a loop that
     traps must trap at the same iteration with the same message, so a
     possibly overflowing body falls back to the scalar loop for the
     block where a lane would overflow (check per vector step, rerun the
     step scalar).
   - Vector types: the wide class of 0323 generalized from two doubles
     to 16 bytes, 8 halves, 4 words, 2 Ints; the MIR's `$q`/`$v`
     templates carry an arrangement; spill slots exist (16 bytes).
   - x86_64: the wide class does not exist there (slp_f64 returns on
     x86); xmm0 to xmm13 would need the same allocation class, SSE2 only
     by default (gotchas/amd64-baseline-sse2.md), wider forms behind
     cpu.hw bits with a scalar fallback (TestX86SIMDGated).
   - Both encoders: arm64's NEON forms are in compiler/macho op.xo one
     family at a time (each word checked against clang in
     macho_test.xo); x86_64's SSE forms in enc_amd64.xo.
   - Code size: each vectorized loop keeps its scalar loop for the tail
     and the fallback, plus a vector body and a trip count check. On
     the programs measured, clang's vectorizers changed binary size by
     under 0.1 percent because few loops qualify; a native vectorizer
     of the same reach would cost as little and gain as little.

## Update (2026-11-07): step 1 results

Decision step 1 (the intrinsic route for search shapes) went on after
this ADR was written:

- Step 3b, the JSON string scan (pushed cd76d3822): rt.bytes_any_stop32
  (a quote, a backslash, a control byte or a byte of 0x80 and above in 32
  bytes, one signed compare for the last two) on arm64, x86_64, LLVM and
  wasm; jsontext str_end and append_body skip 32 clean bytes a step.
  bench/perf/json enc_long and long_strings: see std/changes/json.md
  (2026-11-06, xo bench --ab on host 2, 8 rounds per order); same
  position ratios 0.86 to 0.94 for enc_long. A fused class intrinsic,
  as this ADR's step 1 predicted ("a class intrinsic ... one more code
  generation step and seed").
- Step 3a: the gated 32 byte probe (count_byte, one byte split,
  find_bytes) was measured and dropped (ab3e.txt, host 1: count_nl 1.292,
  find_rare 1.209, find_common 0.516): a probe that fails costs a block,
  so it paid only on sparse bytes. find_bytes keeps the first and last
  byte filter behind a cutover of false starts (4 plus one per 64 bytes;
  Go's one per 16 never fired on a first byte one in 26).
- Step 3c (rt.bytes_count_eq32, a 32 byte count with no branch): stage 1
  (code generation) pushed; stage 2 (count_byte uses it at every
  density) with 3a's find_bytes in one push (linux/arm64 host 1, 5
  rounds, against main with the workloads): count_nl 0.741, count_dense
  0.869, count_rare 0.71; find_common 0.347, find_rare 0.997 (cutover at
  one false start per 64 bytes).

What this says about step 2 (idiom recognition) and step 3 (a loop
vectorizer):

- Each search kernel so far needed its own lane shape (any_eq32,
  any_high32, mem_ne32, any_stop32, count_eq32): a general vectorizer
  would have to synthesize the same mask, early exit and locate
  sequences, which clang 23 does not do on these loops (the ceiling
  table). The hand kernels remain the cheaper route for class C.
- A fixed kernel set also carries a cost the ADR did not list: every new
  intrinsic needs a seed before std can use it (two pushes and a seed
  per kernel; json32 waited for 0.43.0). Step 2's idiom pass, which
  calls std kernels written with existing intrinsics, has no seed cost
  unless it adds a lane shape.
- Two wasm failures came from the WAT assembler's opcode table lacking
  instructions a new lowering used (gotchas/wasm-assembler-opcode-table.md,
  third hit with any_stop32's i32.extend8_s);
  internal/repotest TestWasmAsmOpcodes now checks the table against every
  mnemonic the wasm lowering emits. Any vector kernel added for step 2
  gets the check for free.

Proposed next measurement for step 2 (unchanged from the ADR, now with a
concrete first target): websocket's mask xor and a count of UTF-8
continuation bytes, hand kernels against the native loop, on host 1 with
`xo bench --ab`; the 0366 bar (2 times on the loop alone) decides.

## Update (2026-11-07): step 2's kernel-vs-loop measurement

The bar is decision 0366's: a hand kernel must beat the native loop by 2x
on the loop alone (kernel/loop at or below 0.5), or step 2 stops at that
target. Measured on linux/arm64:

| Target | Loop (std) | Hand kernel | Kernel/loop | Bar |
|---|---|---|---:|---|
| count of UTF-8 continuation bytes (scan.xo count_cont) | 8 bytes a step, SWAR | NEON, 16 a step, a byte counter widened every 255 steps (bench/perf/vecstep2, `use c`, the call's cost subtracted) | 0.46 (host 2), 0.42 (host 1: loop 38 ms, kernel 17 ms, each less the empty call's 2 ms; medians of 7 runs) | passes |
| websocket mask xor (websocket.xo mask_to) | a word a step | arm64 eor of two q registers, 32 bytes a step (a scratch intrinsic, never pushed; bench/perf/websocket big as an --ab) | not separable: the program 1.012 of main (0.813 to 1.081, 10 rounds, host 1), within noise; the loopback round trip is the cost | stops (no gain where it lives) |

Two step 1 intrinsics that wait for a seed were measured before asking
for one (ab-host builds B's xo with A's, which already lowers them, so no
seed is needed to measure); neither gained, so both std uses stay held
and no seed was requested:

| Intrinsic (on main, code generation only) | Held std use | Workload | New/base |
|---|---|---|---:|
| rt.mem_ne64 (f2efe635e) | memcmp's 64 byte step as one test (lang1-mc64b) | simdscan equal, compare | 0.981, 0.981 (within noise) |
| rt.mem_any_eq32 (50510c1ba) | http1 head_len's newline scan 32 bytes a step (lang1-memb) | bench/perf/httphead, on the held branch (pipelined GETs, 8 to 20 header lines of 20 to 100 bytes) | 0.993 (within noise) |

A user program cannot express the mask kernel (no word loads, and `use c`
copies a Bytes argument), so it is measured where it lives: a scratch
branch whose std calls the kernel, against main. A plain C loop is no
bound for the count: gcc at -O2 (the linux host's cc) does not vectorize
a loop of unknown trip count, and it read 1.9 times slower than std's
SWAR loop; the kernel is NEON intrinsics by hand.

Result: the count passes the bar on both hosts (0.42 and 0.46), so
step 2's first kernel is a lane count of continuation bytes for
scan.xo count_cont (an intrinsic as step 1's, 16 bytes a step with a
widened byte counter), which waits for the owner's go as the
Consequences say. The mask stops at step 2: its loop is not where
bench/perf/websocket's time goes.

The count's kernel is rt.bytes_count_below32(b, i, lo) (2026-11-07): the
bytes of 32 below lo as signed bytes (lo -64: continuation bytes), two
signed lane compares and a lane sum (arm64 cmgt and addv, x86_64 pcmpgtb,
pmovmskb and popcnt, LLVM a <32 x i8> compare and ctpop, wasm a byte
loop). It lands as code generation first; scan.xo count_cont's use of it
(lang1-ccont-std) waits for a seed that lowers it. Measured before then
(ab-host builds the use with the code generation commit's xo):
bench/perf/simdscan rune_count 0.502 of the code generation commit
(0.485 to 0.537, n=6, host 1). It is an intrinsic, not a pass: no other
loop changes, so rt.ct_barrier code is untouched, and a fault in
count_cont's loop is impossible (each block is in bounds by the loop's
test), as before.

The remaining step 2 candidates (2026-11-07, host 1, --ab against main):

| Candidate | Change | Result |
|---|---|---|
| html escape sizing | five rt.bytes_count_eq32 a 32 byte block (2965673e9) | 0.871, pushed |
| find_byte 32 byte steps | rt.bytes_skip32(b, i, to, c): the 32 byte steps of find_byte as one kernel (arm64 9 instructions a step where the loop around rt.bytes_any_eq32 took 15; x86-64 SSE2; LLVM an IR loop; wasm a loop over $bytes_any_eq32) | with find_byte on it (branch lang1-skip32, 0ff2882ae): simdscan index_byte 0.912 (0.857 to 0.948, n=6), index_byte_64 0.889 (0.86 to 0.899, n=6), find_rare and find_common within noise; the codegen pushed, find_byte moves after the next seed (std/native/scan.xo is in the self build) |
| png up-unfilter | the row buffer as a Bytes written by address, the up filter a word at a time | 0.975 (0.962 to 0.985); dropped: std/png also builds on the Go backend from std/native/png.xo (std/go_compiled.txt), which has no raw addresses or rt intrinsics, and inflate is most of decode (branch lang1-pngup) |
| SHA-256 over many blocks | cpu.sha256_blocks: one loop with the state in registers (arm64 SHA2, x86-64 SHA-NI), where each block was a call that saved and packed the state | simdscan sha256 0.884 (0.871 to 0.894, n=6); x86-64 (amd64 host, SHA-NI) 0.79 of main by medians of 9 runs; pushed |
| SHA-1 over many blocks | cpu.sha1_blocks: one loop with abcd and e in registers (arm64 SHA1, x86-64 SHA-NI), as SHA-256 | simdscan sha1 0.884 (0.873 to 0.899, n=6), perf/sha1 (sums and HMAC-SHA1) 0.977 (0.973 to 0.984); x86-64 (amd64 host, SHA-NI) 74 to 64 ms, 0.86 of main by medians of 9 runs; pushed |

## Consequences

- Step 1 is in std and the emitters already; steps 2 and 3 each need
  the owner's go after their measurement.
- No change to Float results: lanes round as scalars (0287); reductions
  over Float keep their order (no reassociation), so a Float sum does
  not vectorize except as 0323's pairs. Int reductions (wrapping or
  proven not to overflow) do.
- Intrinsics used by std code that xo builds from need a seed that
  knows them (gotchas/seed-older-than-intrinsics.md): every new kernel
  lands in two steps, code generation first.

## Alternatives

- A general loop vectorizer first (LLVM's LoopVectorize shape): the
  largest change (types, legality, cost model, both encoders) for loops
  that are mostly search shapes LLVM does not vectorize either.
- Leave it to the LLVM backend (--backend=llvm for hot programs): it
  vectorizes class A and B today, at LLVM's build time and code size.
- Intrinsics only, no idiom pass: every new hot loop needs a std edit,
  but std is where the hot loops are.
