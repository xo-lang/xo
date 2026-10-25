# A64 has no byte-immediate arithmetic or logical lane operations

Status: Observed (2026-10-25, decision 0373's case fold).

## Symptom

A NEON lane fold written the way the SWAR word version reads does not
assemble. Each of these is `error: invalid operand for instruction` with
clang 16 to 18 for arm64:

```
sub v30.16b, v30.16b, #0x41
add v30.16b, v30.16b, #0x41
and v31.16b, v30.16b, #0x20
orr v31.16b, v30.16b, #0x20
bic v31.16b, v30.16b, #0xdf
```

`movi vD.16b, #imm8` does assemble, and so do the three-register forms.

## Cause

ASIMD's arithmetic immediate forms carry the immediate inside an
8-bit-element encoding slot that only the "modified immediate" family
uses, and for `.16b` the assembler accepts it in `movi` and `mvni` only;
`add` and `sub` have no immediate form at all for 8-bit elements. A
`0x20` byte constant therefore has to live in a register.

## Fix

Keep the constants in registers and use register forms throughout. The
case fold (decision 0373) holds `0x41` in v29 across the loop and builds
`0x20` per matching byte from the compare mask with two byte shifts
(`ushr #7` then `shl #5`) instead of an `and` with a constant, which is
what makes it fit in the three scratch registers (v29 to v31,
compiler/arm64/regalloc.xo). The alternative, three constant registers,
does not exist: the register allocator may use d0 to d28.

Verify an emitted instruction against clang rather than trusting the
manual: `clang -c -arch arm64` a two-line `.s` and read the word with
`otool -s __TEXT __text`, which is what compiler/macho/macho_test.xo's
case fold rows record.
