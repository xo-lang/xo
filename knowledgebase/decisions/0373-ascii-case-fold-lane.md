# 0373: a 16 byte ASCII case-fold intrinsic for the native backends

Status: Proposed, step 1 built (DS-1, 2026-10-25; the codegen, the four
lowerings and the codegen pattern; measurement and landing pending, in
two steps because of the seed, see "Landing").

## Context

Decision 0366 measured the shape this fixes and stopped at its own bar.
Its sweep (main 9706c67e4, 2026-10-24) had unicode upper at 1.956 and
lower at 1.914 times LLVM's instructions; on today's main the same rows
are 1.755 (lower) and 1.696 (upper) of LLVM's, the largest gaps left in
bench/perf/unicode, and the two rows where native is behind LLVM while
the mixed rows (lower_u 0.824, upper_u 0.849, eqlower 0.843) are ahead.

The fold in obj.fold_copy_lo/up was eight bytes a step of SWAR
arithmetic (15 instructions per word in the emitted arm64 code, loop
overhead included, 1.875 a byte); clang vectorizes the same loop 64
bytes a pass and reaches about 0.8 a byte. The sampling profile puts
fold_copy at 43 percent of the lower row's time, and the instruction
count puts it at 61 percent of the row's 3.05 instructions a byte,
which is the ceiling any fold change can reach on that row.

0366's step 1 built the two-lane loop by hand in a C harness: 0.419 of
native's instructions and 1.71 times its wall on the loop alone. The
instructions cleared 2.4 times; the wall did not reach the 2 times the
plan required, so the widening pass was not built. The gap between those
two numbers is the fold's memory traffic, and it is why this decision
asks for an instruction bar and reports the wall honestly.

What arm64 offers shapes the answer (verified with clang 16 to 18,
knowledgebase/gotchas/arm64-no-byte-immediates.md): only v29, v30 and
v31 are outside the register allocator's d0 to d28, and A64 has no byte
wide add, sub, and, orr or bic with an immediate; only movi and mvni
take an 8 bit broadcast. A lane therefore keeps one constant in a
register, tests the range with one unsigned compare of the bytes shifted
down by the case's first letter, and builds 0x20 a byte from the mask
with two byte shifts (ushr #7, shl #5): nine instructions for 16 bytes.

## Decision

One intrinsic, `rt.case_fold16(dst, src, i, mode)` (std/native/rt.xo),
folds the 16 bytes at src from i into dst from i, mode 1 lowering and
mode 2 raising, both checked by the caller and both constants at the
call site. Every byte outside the case, and every byte at or above 0x80,
is copied unchanged. std's fold_copy_lo/up call it 16 bytes a step
before their eight byte and byte loops, which stay for the tail, so the
change is one loop in each of two functions.

The lowerings: arm64 the nine instruction lane above; x86_64 the same
fold with SSE2 (pminub for the unsigned compare, pxor for the case bit,
17 instructions a lane, constants broadcast from r11d because the
emitter has no constant data section and xmm14/xmm15 are its only
scratch); LLVM the same two eight byte SWAR words, so clang sees the
loop it vectorizes today; wasm a byte loop.

## Consequences

- Instructions, expected: the lane replaces the fold's 1.875
  instructions a byte, 61 percent of the row's 3.05. At nine
  instructions a lane plus the std loop's step, about 0.75 a byte, the
  fold falls to 24 percent of the row and the row to about 0.63 of
  today.
- Wall, expected: nearer 1.3 to 1.4 times on the row than the
  instructions suggest, for 0366's reason: the fold is limited by its
  memory traffic, not by its instruction count.
- LLVM's row should not move: its expansion is the same words it
  compiles today, and the bar is that it does not regress.
- The lane's correctness rests on one property: for every byte value,
  `(c - first) mod 256` leaves the case range 0 to 25 exactly for the
  case's letters (0x00 to first-1 wrap to 2^8-1 and above, 0x80 and
  above land at 0x3f and above), so no separate high-bit test is needed.
  scripts/stdcorpus/case.go pins it with Go's results on the lane's
  boundaries and on seeded mixed case texts.
- A 32 or 64 byte lane was refused: three scratch registers cannot hold
  two data lanes and the two constants a lane needs.

## Landing

In two steps, because the intrinsic is used by std/native/llrt/obj.xo,
which xo itself is built from (knowledgebase/gotchas/seed-older-than-intrinsics.md):
the backends and the declaration first, which any seed builds since no
std code calls it yet, so the next seed carries the ability to lower it;
then the std call site, once a seed that knows the intrinsic exists
(owner approved 0.31.0 for 2026-10-25).

- Step 1 shipped with two faults in its LLVM expansion, both fixed in the
  commit that adds this note. It passed the two raw addresses (i64
  operands) into a getelementptr's ptr operand, so clang refused the IR of
  any caller ("defined with type 'i64' but expected 'ptr'"); both are
  inttoptr'd through g.raw_at now, as rt.mem_load and rt.rs_put_word
  already do. And it wrote its two masks as hexadecimal literals, which
  LLVM's IR reads as floating point constants ("floating point constant
  invalid for type"), with 0x8080808080808080 also too large for a
  positive i64; both are decimal now. Step 1's gate could not see either
  because nothing in main calls the intrinsic, so no test built an LLVM
  caller: the first caller is step 2's std call site, and its first LLVM
  build found both, one clang error at a time.

## Alternatives

- 0366's widening MIR pass: a general loop shape recognizer with a wide
  vreg class and a register allocator change, measured at 1.71 times wall
  on the loop against a 2 times bar. This decision asks for the same
  instruction win through one operation instead, with no pass.
- A std-side SWAR rewrite with no intrinsic: the eight byte SWAR loop is
  what std has; without an intrinsic the native backend cannot emit a
  lane, which is the whole gap. A std-side 16 byte unroll of the same
  words was measured on 2026-10-25 (bench/results/case-swar-2026-10-25.md):
  arm64 14 percent fewer instructions on the row, LLVM 1.7 percent more,
  so it was not landed.
- Widening the existing loop inside the compiler by recognition: what
  0366 proposed and measured; refused at its bar and refused again here
  for the same reason (it must guess which loops are safe).
