# 0385: An explicit fused multiply-add, Float.fma

Status: Accepted (owner, 2026-10-26: explicit FMA only, no implicit
fusing; written up by Xo Lang 2, 2026-10-26). Stage 1 built
(2026-10-26); stages 2 and 3 to do.

## Context

Every Float operation in Xo rounds, on every backend (decision 0287):
`a * b + c` is a rounded product and a rounded sum, and golden
float_no_fma fails if a backend fuses them (an arm64 fmadd peephole,
LLVM's `contract` flag, Go's compiler fusing on arm64). That keeps
results bit for bit identical across backends and architectures, but it
leaves out the one operation numeric code wants fused on purpose: a
multiply-add rounded once, which is more accurate (error-free
transformations, compensated sums, polynomial evaluation, dot products)
and on arm64 one instruction instead of two.

The owner chose an explicit operation only (2026-10-26): code asks for a
fused multiply-add by name, and nothing written as `a * b + c` is ever
fused.

## Decision

**Surface.** A prelude method on Float:

```
/// f * y + z computed exactly and rounded once (IEEE 754
/// fusedMultiplyAdd; Go's math.FMA): the same bits on every backend and
/// architecture.
fn (f: Float) fma(y: Float, z: Float) -> Float
```

Special values are IEEE 754's (and Go's math.FMA's): a NaN operand gives
NaN; `inf * 0 + z` is NaN; `inf * y + (-inf)` with a finite nonzero y of
the same sign is NaN. The product is never rounded, so a product that
would overflow or underflow on its own is carried exactly into the sum.
The sign of an exact zero result follows IEEE 754 under round to
nearest: `+0` unless both the exact product and z are `-0`.

No implicit fusing anywhere: float_no_fma stays as it is, and `a * b +
c` keeps two roundings on every backend.

**Lowering.**

| backend | lowering |
|---|---|
| native arm64 | `fmadd d, a, b, c` (every ARMv8 CPU has it) |
| LLVM, arm64 target | `@llvm.fma.f64` (selected as fmadd) |
| native amd64, LLVM amd64 | stages 1 and 2: the software body; stage 3: `vfmadd231sd` when CPUID says FMA and the OS saves AVX state, else the software body |
| wasm | the software body (core wasm has no scalar fused multiply-add; relaxed-simd's madd may or may not fuse, so it is not used) |
| Go backend | `math.FMA` (exact in Go on every architecture: an instruction or Go's software path) |

The software body is a port of Go's math/fma.go (std/native/llrt/
gomath.xo, beside the other ports of decision 0287): split both operands
into sign, exponent and 53 bit mantissa, multiply into 128 bits (Go's
bits.Mul64 as 32 bit halves), align and add z's mantissa with a sticky
bit, normalize, and round to nearest even once. About 120 lines; the
128 bit pieces are wrapping Int arithmetic
(gotchas/std-port-costly-go-idioms.md).

LLVM on amd64 does not get `@llvm.fma.f64` in stages 1 and 2: without
`+fma` LLVM lowers it to a call of libm's `fma`, and Xo links no C
library (zero C runtime); the call goes to the software body instead.

**Stages.**

1. The method, the software body on every native and wasm build, math.FMA
   on the Go backend; spec/std.md; the tests below.
2. arm64: Float.fma lowered to fmadd in the native emitter and to
   `@llvm.fma.f64` in the LLVM emitter for arm64 targets. What it needs:
   - compiler/ssa call.xo: `Float.fma` joins the intrinsic keys (beside
     Float.sqrt) only when `b.goarch == "arm64"`, as the AES and ChaCha
     intrinsics are gated, so wasm, the amd64 emitters and an arm64
     build's Go backend keep calling the software body (float_fma in
     std/native/prelude.xo) and need no change;
   - compiler/arm64 calls.xo: `"Float.fma" => g.def1(true, "fmadd $d0,
     $u0, $u1, $u2", [x, y, z])` (fmadd d, n, m, a is n * m + a);
   - compiler/macho op.xo: fmadd's encoding (floating-point
     data-processing, 3 source, double: 0x1F400000 | Rm << 16 | Ra << 10
     | Rn << 5 | Rd) in the FP word encoder beside fmul;
   - compiler/arm64 rle.xo's pure op list and slp.xo (fmadd is a scalar
     op the SLP pass leaves alone at first);
   - compiler/llvm call.xo: `call double @llvm.fma.f64(double, double,
     double)` and its declare in gen.xo, for arm64 targets only;
   - compiler/check realtime.xo: Float.fma in the list of calls that do
     not allocate (it never does, on any path);
   - the float_no_fma pattern in test/codegen keeps saying no fmadd for
     `a * b + c`, and a new pattern says `x.fma(y, z)` is one fmadd with
     no call.
   It changes code generation, so it is gated as a codegen change (the
   arm64 and LLVM goldens, the codegen patterns; no x86 change).
3. amd64 hardware: cpu.xo gets HW_FMA (CPUID leaf 1 ECX bits 12 FMA, 27
   OSXSAVE and 28 AVX, and XGETBV's XCR0 bits 1 and 2, a new cpu
   intrinsic), and Float.fma becomes a branch on hw() between
   `vfmadd231sd` (a VEX encoding in compiler/macho enc_amd64.xo) and the
   software body, as the AES and SHA-NI paths do (decision 0231). Gated
   by the Docker amd64 sample, and a run under `qemu -cpu` without FMA
   for the fallback.

## Tests

- Golden float_fma (internal/llvm testdata, run by every backend's
  golden runner): cases where the fused and the unfused results differ
  (`0.1.fma(10.0, -1.0)` is 5.551115123125783e-17 where `0.1 * 10.0 -
  1.0` is 0; `(1 + 2^-30).fma(1 - 2^-30, -1.0)` is -2^-60, about
  -8.67e-19, where the unfused form gives 0), signed zeros, NaN and the
  infinities, a product that overflows and one that underflows to
  subnormals, z far larger and far smaller than the product, and halfway
  cases of the single rounding; want.txt from Go's math.FMA.
- A corpus of 20,000 cases inside float_fma: a generator (an LCG in the
  program) draws signs, mantissas and exponents so that the product and
  z overlap (every 97th z subnormal or zero), and the program prints a
  digest of the results; want.txt is the Go backend's output, which is
  Go's math.FMA, so every other backend must match Go bit for bit on all
  20,000 (2,350 of them differ from two roundings). The same golden
  checks each hardware lowering as it lands.
- float_no_fma keeps passing on every backend: the new op never comes
  from `a * b + c`.
- test/codegen: `x.fma(y, z)` is one fmadd on arm64 (stage 2) and
  vfmadd231sd behind a hw() branch on amd64 (stage 3); float_no_fma's
  pattern still says no fmadd.

## Consequences

- One more prelude method; no new syntax, no new diagnostic.
- The native backends get one more three operand float op (stage 2);
  the register allocator already handles three sources (madd, msub).
- Until stage 3, Float.fma on x86-64 costs a software call (Go's
  software path is a few dozen instructions); arm64 is one instruction
  from stage 2.
- The prelude method is std code that the compiler lowers from stage 2,
  so a seed older than stage 2 builds a program using it with the
  software body: correct, only slower. No seed cut is needed
  (gotchas/seed-older-than-intrinsics.md), as the body stays as the
  fallback.

## Alternatives

- A free function `fma(a, b, c)` (the owner's shorthand): Xo puts the
  math functions on Float as methods (decision 0287: sqrt, pow, hypot,
  atan2), and the prelude has no free math functions; `a.fma(b, c)`
  reads as `a * b + c`. The operation is the same; the method form was
  chosen for consistency.
- Implicit fusing (LLVM's contract, an fmadd peephole): ruled out by
  decision 0287 and by the owner's choice; results would differ between
  backends and between optimized and unoptimized builds.
- A Float32 fma: Go has no float32 FMA to check against; later, when a
  caller needs it (the same body works at 24 bits).
- Hardware only on amd64 (fault on a CPU without FMA): every x86-64 CPU
  since Haswell (2013) has it, but Xo's amd64 baseline is v1, and a
  program must not fault on an older CPU or under an emulator that hides
  it.
