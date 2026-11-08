# 0361: Float32 arithmetic in single-precision registers on the native backends

Status: Accepted (lead, 2026-10-23, as a codegen change: the results stay
bit-identical). Stage 1 landed 2026-10-23: portable float32 0.799 of the
instructions before it on linux/amd64 (99,714,356 to 79,714,747), now
0.945 of LLVM's; golden f32_bits identical on arm64, LLVM and the Go
backend.

## Context

Decision 0101 made Float32 a double holding a float32 value. The SSA
builder rounds every `+ - * /` on Float32 back to float32 with
`rt.f32_round` (compiler/ssa/expr.xo), converts Int with one rounding
(`rt.i64_to_f32`), and Float with one rounding (`rt.f32_round`). The
native backends lower `rt.f32_round` as `fcvt s, d` then `fcvt d, s`
(x86_64: `cvtsd2ss`, `cvtss2sd`), so each Float32 operation is three
instructions: the double operation and two conversions. LLVM's
backend gets the same SSA, but clang's optimizer folds `fptrunc(fadd
(fpext a, fpext b))` into a single-precision `fadd` and keeps the
values in single-precision registers.

Measured, callgrind linux/amd64, the same xo and std on both backends
(2026-10-23): portable float32 is 101,714,223 instructions native
against 84,372,275 for LLVM (1.206). That loop does five Float32
operations and two conversions an iteration. The other portable
workloads do no Float32 arithmetic, so they do not change. LLVM's
count is the bound for this change. The native gain is estimated from
it (about 0.83 of today's float32 instructions) until stage 1 exists to
measure.

## The question: can any result change?

No. Every value and every bit stays as 0101 defines it:

1. **Each operation.** A Float32 `+ - * /` computes the double result
   of two float32 values and rounds it to float32. Doing the operation
   in single precision gives the same float32, bit for bit: for a
   binary format of p = 24 bits, rounding the exact result first to q
   bits and then to p bits equals rounding it once when q >= 2p + 2
   (Figueroa, "When is double rounding innocuous?", 1995; q = 53 >=
   50). This holds for round to nearest even on all four operations
   (and square root), including results that are subnormal in float32,
   since the double result of two float32 operands is either exact or
   correctly rounded to 53 bits first. Neither native backend sets
   flush-to-zero or denormals-are-zero (both run with the default
   FPCR and MXCSR), and this change must keep it so.
2. **Widening and narrowing.** A float32 value converts to double
   exactly and back exactly, so where a Float32 meets a double (text,
   `Float(x)`, a comparison with 0101's double compare, a call that
   passes it as a double, a store), widening the single value gives the
   double 0101 holds today.
3. **NaN bits.** `Float(x).bits()` can show a NaN's payload. A default
   NaN (`0.0 / 0.0`, `inf - inf`) is 0x7FC00000 in single precision on
   arm64 and 0xFFC00000 on x86_64. That is exactly what the double
   default NaN (0x7FF8..., 0xFFF8...) narrows to. An operation on a NaN
   operand propagates that operand's payload in both precisions (arm64
   FPCR.DN = 0, x86_64 SSE's first-operand rule), and widening and
   narrowing keep the payload's high bits, which are all a float32
   payload has. The stage 1 golden checks these bits on both
   architectures.
4. **What does not move.** Literals, conversions (Int, Float, UInt64),
   comparisons, text, `Int(x)`, and memory layout stay as 0101 and the
   SSA builder define them. This change is only how the native backends
   compute `round32(a op b)` and where a value lives between operations.

Go's float32 semantics round every operation to float32, as above. Go
also permits fusing `x*y + z` into one rounding. The Go backend compiles
to Go, so `gc` may fuse on arm64 where native and LLVM do not. That is
a parity question that exists today for both Float and Float32, and
this decision does not change it.

Found while checking this: `Float32(u)` for a UInt64 above 2^53 rounds
twice, to a Float and then to float32 (compiler/ssa/call.xo
`u64_to_float` then `f32_round`). That differs from Go and from Xo's
own Go backend: `Float32(u)` for the UInt64 u = 2^60 + 2^36 + 1 gives
2^60 natively and on LLVM, but 2^60 + 2^37 on the Go backend and in Go.
This bug predates this decision and gets its own fix: one rounding,
through `i64_to_f32` of `u >> 1 | u & 1`, then times 2, as the UInt64
to Float path does.

## Decision (proposed)

The native backends (compiler/arm64, and x86_64 through it) keep a
Float32 value that a Float32 operation produced in a single-precision
register. They compute `rt.f32_round(a op b)` on Float32 operands as the
single-precision operation (`fadd s`, `addss`) on the operands' single
forms, and they widen only where a double is needed. Stages:

1. isel: `rt.f32_round` of a Float32 `Add | Sub | Mul | Div` whose
   operands are Float32 values selects the single-precision operation
   on their single forms. A value's single form is the register a
   previous single operation defined, a narrowed constant, or a
   `fcvt s, d` of its double. The double form is made on demand with
   `fcvt d, s` and shared through the memo, as constants are. Golden:
   the float32 golden plus a new one with NaN payloads, signed zeros,
   subnormals and overflow, checked bit for bit against LLVM and Go
   parity. Measure portable float32 with callgrind and one `--ab`.
   Landed with two more pieces: the conversions to Float32
   (`rt.i64_to_f32`, `rt.u64_to_f32`) keep their single form, and LICM
   hoists `fcvt` (a loop-invariant Float32 operand narrowed once).
   f32_bits: the edges of float32 (zeros, subnormals, the largest
   finite, infinities, default and payload NaNs) through `+ - * /` and
   a chain, bit for bit. With two NaN operands only the NaN flag is
   checked, since IEEE 754 does not fix which payload survives and Go's
   compiler may swap a commutative operation's operands.
2. Loop-carried Float32 values (block parameters) stay single across
   the back edge, so a loop accumulator never widens. Landed
   2026-10-23: a loop header's Float32 parameter gets a single-precision
   register that edge_moves and param_moves fill (the argument's single
   form, or narrowed in the predecessor). select_branch, range_and and
   logic_value, which build their own parameter moves, leave such a
   block alone. Portable float32 79,714,747 to 77,714,748 instructions
   (0.92 of LLVM's). Golden f32_loops: branches, nested loops, a
   Float32 loop condition, break, and a select.

## Consequences

- No program output changes (section "The question"), so no spec change
  and no owner decision is needed for the representation. The UInt64
  conversion fix changes printed results and goes in separately, with a
  std or spec note.
- Single-precision instructions on Float32 chains: about two fewer
  instructions per operation. LLVM's code bounds the gain at about 0.83
  of portable float32.
- More register forms to track in isel (single and double of one
  value). A missed widen would be a miscompile, so each stage gates on
  the Docker amd64 sample and the new golden.

## Alternatives

- Keep 0101's lowering: three instructions per operation, float32 stays
  1.21 times LLVM.
- A MIR peephole over `op d; fcvt s, d; fcvt d, s` chains: it sees no
  values across blocks and misses loop-carried accumulators, which is
  where the time is.
- Change 0101's representation (Float32 as a 32-bit value in memory and
  in calls): touches layouts, the C ABI, and every backend for the same
  instruction count in loops. Not needed for the speed.

## Addendum (2026-11-08): riscv64

RISC-V's F and D extensions never propagate a NaN payload: every NaN
result is the canonical NaN (0x7FC00000 in single precision,
0x7FF8000000000000 in double), whatever the operands' payloads. So on
linux/riscv64 (decision 0571, LLVM only) point 3's propagation does not
hold, and golden f32_bits' hash differs there for exactly the cases with
one payload NaN operand. As point 3 already treats a NaN's bits as the
hardware's (the default NaN's sign differs between arm64 and x86_64),
this records a hardware fact, not a semantics change (lead, 2026-11-08).
f32_bits keeps riscv64's run in want_linux_riscv64.txt: the golden
runners take want_<goos>_<goarch>.txt before want_<goos>.txt and
want.txt (internal/llvm wantFileFor). Propagating payloads in software
(a class test and a select per operation) was rejected: it would tax
every float operation for a property spec/core.md does not promise.
