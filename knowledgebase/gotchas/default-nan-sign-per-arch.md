# A default NaN's sign is the architecture's

Status: Observed (2026-10-23)

**Symptom.** A golden that prints the bits of `0.0 / 0.0` (or
`inf - inf`) passes on arm64 and fails in the Docker amd64 run:
`7ff8000000000000` on arm64 against `fff8000000000000` on x86_64, and a
hash over such bits differs in its top bit. Old and new compilers print
the same bits, so it is not a code generation change.

**Cause.** An invalid operation produces the hardware's default NaN.
arm64's is positive (0x7FF8... in double, 0x7FC00000 in single), and
x86_64 SSE's "real indefinite" is negative (0xFFF8..., 0xFFC00000).
Propagated NaNs keep their operand's payload on both. With two NaN
operands, which payload survives is not fixed either: native and LLVM
keep the first, and Go's compiler may swap a commutative operation's
operands.

**Fix.** A test that checks Float bits compares a NaN made from
non-NaN operands, and a result of two NaN operands, by `is_nan()`
only. It compares the bits of a NaN propagated from one operand (golden
f32_bits, decision 0361).
