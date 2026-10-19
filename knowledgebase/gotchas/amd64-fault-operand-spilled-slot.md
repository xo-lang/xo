# amd64: an overflow fault printed the sum as its operand

Status: Observed (2026-10-19, Xo lang 4)

## Symptom

progen arith on Xo's amd64 backend (scripts/progen-amd64.sh, seeds 74,
111, 122, 123, 145, 174 of 1..250) exited with the right status and
output, but the integer overflow message named an operand 2^64 off:
Go's `2147483650 + 9223372036854775804 does not fit Int`, amd64's
`-9223372034707292162 + ...`, the wrapped sum.

## Cause

`acc += x` in the MIR (not SSA) reads and writes one register, acc.
With acc spilled, the checked add became `movq SLOT, %r11; addq x,
%r11; movq %r11, SLOT; jo stub`: the result's store comes before the
overflow branch, and the stub read the operand from SLOT. The stub's
recovery of an operand the result overwrote (x86emit.xo
x86_recover_operand, the arm64 emitter's recover_operand) handled only
a result in a register.

## Fix

x86_recover_operand recomputes the operand (the result minus the other
operand, for an add) when the result's spill slot is the operand's too
(x86_clobbered compares slots). Regression golden:
internal/llvm/testdata/golden/overflow_spilled_operand (seed 74), run on
every backend; run the amd64 goldens in Docker
(scripts/docker-goldens.sh) for a change to the x86 fault paths.
