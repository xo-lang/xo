# A value kept in memory must be read before another store reuses that memory

Status: Observed (2026-10-10)

## Symptom

The first build of decision 0148 (values of `var` parameters kept in the
caller's memory) failed the byref_struct golden: after
`swap_names(p, q)` both structs printed `q: 11 items ...`, and 7
allocations leaked.

## Cause

`swap_names(var x: Big, var y: Big)` calls `x.add(100)` and then
`y.add(200)`. The home slot of type `Big` (decision 0145) is `x`'s
memory, so `y.add` stores `y` into `x`'s memory. The dataflow knew that
(after the second call the memory holds `y`'s result) and kept `x`'s
updated value materialized, but `y`'s result was held at its only use,
the return, so it was read lazily from memory there. The return lowered
the write backs in order: first `x`'s value into `x`'s memory, then a
load of `y`'s value from that same memory, now overwritten.

## Fix

Lower every operand of a call, and every write back of a return, before
any store (internal/llvm/fromssa.go `call` and `OpReturn`). The general
rule for code that lowers lazily from memory: an instruction that both
reads held values and writes memory reads first.
