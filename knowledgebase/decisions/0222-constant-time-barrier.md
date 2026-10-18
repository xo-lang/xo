# 0222: Constant time value barrier and select for Xo runtime code

Status: Accepted (B09, 2026-10-14; decision 0195 prerequisite 5, the
zero-C audit's item B09), implemented: `rt.ct_barrier`,
std/native/llrt/ct.xo, crypto.equal and Ed25519 in Xo (the C deleted:
xo_crypto.c 540 lines to 16), and the checks below. Ed25519 runs at
0.944 to 0.971 of the C's instructions on arm64 and 0.757 to 0.768 on
LLVM (bench/results/ed25519-consttime-2026-10-14.md).

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
    self-hosted arm64 emitter's code of ct.mask_nonzero, ct.pick, and
    ed25519.ge_cmov has no conditional branch, csel, or cset,
    ct.bytes_equal no csel, cbz, or tbz, ed25519.ge_mul and sc_reduce
    no csel or cset, x25519.madd_digit and base (the fixed base
    public key, 2026-10-18) no csel or cset; the LLVM IR carries the barrier asm (twice in
    bytes_equal) or calls ct's masks. `xo dev codegen` gained `// func
    MOD.NAME` lines to name a function of another module (a std runtime
    module's).
  - internal/llvm TestConstantTimeLLVM (Go, -short too, about 1 s):
    the IR of a program using std/crypto through clang -O2 for the host,
    the checked functions made external and noinline (their call sites'
    alwaysinline dropped); each keeps its barriers after -O2 (the asm
    markers in the output) and has no conditional branch and no select
    where the check says (ct.mask_nonzero, ct.bytes_equal,
    ed25519.ge_cmov, sc_reduce, ge_mul). Its control compiles the same
    IR with every barrier an add of 0 and logs the selects: sc_reduce
    gets 4 csel on arm64, which is what the barrier prevents.
    x25519.madd_digit and base are checked outside -short (their
    program reaches them through std/internal's crypto_op, about 80 s of
    clang); adding them found 38 times a carry bit turned into a select
    in x25519.xo's generated field code (now through the barrier,
    scripts/x25519fe64.py mul38) and selects in checked arithmetic's
    fault paths (now wrapping arithmetic). Their control: 20 and 5
    selects without the barriers. On arm64 hosts it checks arm64; the same test
    checks x86-64 on an amd64 host (the LLVM backend builds for the host
    only).
- **crypto.equal is Xo** (std/native/crypto.xo calls ct.bytes_equal, as
  does verify_password in digest.xo); the C `xo_crypto_equal` is
  deleted.
- **Ed25519 is Xo**, std/native/llrt/ed25519.xo, the same results as
  Go's crypto/ed25519: five 51 bit limbs; the elements and points of
  one operation in one workspace List[Int] reached through raw memory
  (decision 0207, constant offsets); a product of limbs as `(f << 13) *
  g`, whose high word is p >> 51 and whose low word shifted right 13 is
  p mod 2^51; a squaring with the 15 distinct products; the doubling
  dbl-2008-hwcd; scalar multiplication by a fixed 4 bit window whose
  table entry a masked scan of all 16 selects; the scalar reduction bit
  by bit with masked subtraction. SHA-512 is digest.sha512_le (by object
  address, so it builds on wasm too). Verification's inputs are public
  and it branches on them (decoding, the canonical S check, the final
  compare). xo_crypto.c's Ed25519, std/native/llrt/cabi_crypto.xo (the
  SHA-512 export the C called), and rtcrypto's Ed25519 declarations are
  deleted: decision 0180's bar holds on both backends, so no C path
  stays.
- internal/llvm TestCryptoXoDiff checks crypto.equal and Ed25519 (RFC
  8032 vectors, Go on random and mutated inputs, non canonical
  encodings, messages at SHA-512's block boundaries) on both native
  backends; TestCryptoDiff keeps the hashes' C reference only.
- **Seed compatibility**: the prebuilt seed lowers `rt.ct_barrier` as a
  call of `xo_ct_barrier`, so xo_crypto.c keeps that function (an empty
  asm) until the seed is a release with this decision
  (gotchas/runtime-c-the-seed-still-calls.md).

## Consequences

- Constant time code can leave C: crypto.equal and Ed25519 did (the
  audit's B09), the TLS stack's primitives may later (decision 0195
  owner decision A).
- Ed25519 now builds on wasm (std/crypto's C Ed25519 did not); wasm
  promises nothing about timing.
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
