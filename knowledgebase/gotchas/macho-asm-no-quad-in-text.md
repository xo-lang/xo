# compiler/macho takes no .quad in __text

Status: Observed (2026-10-18, Xo lang 4)

## Symptom

A change that put a 24 byte constant after each arm64 function (its own
empty list, read by `adr`) built s1 with the seed, then failed at s2
with `xo: assembler: line N: .quad in __text`.

## Cause

Xo's own assembler (compiler/macho asm.xo) lays out code as 32 bit
words. In a code section it takes `.p2align` (padded with nop) and
`.long` (a constant, or a label difference for jump tables), and rejects
every other data directive. The seed's emitter did not have the change,
so s1 (built through the seed) assembled fine; s1's own output was the
first to carry the directive into compiler/macho.

## Fix

Write constants in code as `.long` words, low word first (an 8 byte
value k is `.long k & 0xffffffff` then `.long k >> 32`), after a
`.p2align 3` when the reader needs 8 byte alignment (compiler/arm64
emit.xo local_empty). Test an emitter change that adds directives with
the fixed point, which runs compiler/macho on the new compiler's output.
