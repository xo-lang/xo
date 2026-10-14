# 0222: Constant time value barrier and select for Xo runtime code

Status: Accepted (B09, 2026-10-14; decision 0195 prerequisite 5, the
zero-C audit's item B09), implemented: `rt.ct_barrier`,
std/native/llrt/ct.xo, crypto.equal in Xo, and the checks below.

## Context

Decision 0195 keeps constant time code in C until Xo can promise that
the compiled code has no branch, and no load address, that depends on a
secret. xo_crypto.c held crypto.equal (a `volatile` accumulator) and
Ed25519 (masked selection in the scalar ladder) for that reason.

Two things can break constant time code written in a high level
language:

1. The language's own checks. Xo's Int arithmetic is checked: `a + b`
   is an add and a branch on overflow, an index is a bounds check. A
   check on a secret value is a secret dependent branch, even when it is
   never taken.
2. The optimizer. LLVM recognizes `b ^ (m & (a ^ b))` with `m = 0 - (bit
   & 1)` as `select bit, a, b` (observed 2026-10-14, Apple clang 21 -O2:
   `tst; csel` on arm64, `testb; cmoveq` on x86-64; with `asm("" :
   "+r"(m))` after the mask, `sbfx; bic; and; orr` and `neg; xor; and;
   xor`); on x86-64 the cmov conversion pass may turn a
   select inside a loop into a branch, and any target may turn a loop
   whose accumulator saturates into an early exit. Go relies on its
   compiler not doing this; C code uses `volatile` or an empty asm
   (BoringSSL's `value_barrier_w`, `asm("" : "+r"(a))`).

The self-hosted arm64 emitter does neither: it emits each SSA value as
written, with no if-conversion or range reasoning across calls.

## Decision

- **One intrinsic, `rt.ct_barrier(x: Int) -> Int`** (std/native/rt.xo,
  internal like every `native/rt` function, decision 0074): returns x,
  but the compiler may not reason about the result. Lowering: arm64,
  nothing (the result is the operand's register); LLVM, `call i64 asm
  "", "=r,0"(i64 %x)` (no `sideeffect`: it may be merged or dropped when
  unused, never folded); wasm, a call of the runtime's identity function
  `$ct_barrier` (wasm promises no timing; the barrier keeps the code the
  same shape). It neither allocates nor faults (selfhost/ssa contract.xo
  lists it for the real-time check).
- **Constant time helpers in Xo**, std/native/llrt/ct.xo: masks are 0 or
  -1; `mask_bit`, `mask_nonzero`, `mask_eq`, `mask_lt` (unsigned, the
  borrow of bits.Sub64), `pick(m, a, b)` (a when m is -1: `b ^
  (barrier(m) & (a ^ b))`; `select` is a keyword), and `bytes_equal`
  (subtle.ConstantTimeCompare: words through rt.bytes_word, the
  accumulator through the barrier every iteration so no early exit).
  Every mask leaves its helper through the barrier, so a caller that
  inlines `pick(mask_bit(b), x, y)` cannot let LLVM rebuild the select.
- **Rules for constant time Xo** (the module comment of ct.xo): secret
  values go through bitwise operators, `rt.int_wrap` (wrapping
  arithmetic, not the checked `+ - * << >>`), `rt.u64_mul_hi`, and the
  helpers; never `if`, `match`, `&&`, `||`, a comparison, or an index.
  Lengths and loop counters are public.
- **Checks**, both run by default:
  - test/codegen/consttime (`xo dev codegen`, decision 0217): the
    self-hosted arm64 emitter's code of ct.mask_nonzero has no
    conditional branch, csel, or cset, ct.bytes_equal no csel, cbz,
    or tbz; the LLVM IR carries the barrier asm (twice in
    bytes_equal). `xo dev codegen` gained `// func MOD.NAME` lines to
    name a function of another module (a std runtime module's).
  - internal/llvm TestConstantTimeLLVM (Go, -short too, about 1 s):
    the IR of a program using std/crypto through clang -O2 for the host,
    the checked functions made external and noinline (their call sites'
    alwaysinline dropped); each keeps its barriers after -O2 (the asm
    markers in the output) and has no conditional branch and no select
    where the check says. On arm64 hosts it checks arm64; the same test
    checks x86-64 on an amd64 host (the LLVM backend builds for the host
    only).
- **crypto.equal is Xo** (std/native/crypto.xo calls ct.bytes_equal, as
  does verify_password in digest.xo); the C `xo_crypto_equal` is
  deleted. internal/llvm TestCryptoXoDiff checks crypto.equal (and
  Ed25519 through std/crypto) against Go on both native backends.
- **Seed compatibility**: the prebuilt seed lowers `rt.ct_barrier` as a
  call of `xo_ct_barrier`, so xo_crypto.c keeps that function (an empty
  asm) until the seed is a release with this decision
  (gotchas/runtime-c-the-seed-still-calls.md).

## Consequences

- Constant time code can leave C: crypto.equal now, Ed25519 next (the
  audit's B09 step 3), the TLS stack's primitives later (decision 0195
  owner decision A).
- The barrier costs no instruction on arm64 and none on LLVM beyond
  keeping the value in a register (the empty asm emits nothing).
- What is not enforced: nothing stops a runtime module from branching on
  a secret. The checks cover the helpers and the functions listed in the
  tests; a taint check in the SSA (decision 0195 item 3's "constant time
  marker the SSA builder enforces, a MIR taint check") stays open.

## Alternatives

- A `volatile` load and store of the accumulator (the C's way): needs a
  memory slot per value and costs a load and store per use; Xo has no
  volatile.
- A select intrinsic (`rt.ct_select`) lowered to csel and cmov: one
  instruction, but every emitter must lower it and x86's cmov conversion
  can still turn an LLVM select into a branch unless it too sits behind
  asm; the barrier plus masks is the same code with one intrinsic.
- `asm sideeffect`: stops LLVM from merging two barriers of the same
  value, which no pattern here needs; it would also keep dead ones.
- A noinline call of an identity function on LLVM: a real call per
  barrier.
