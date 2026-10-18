# 0260: Named wrapping and unsigned operations

Status: Proposed (xo-4, 2026-10-18; stub, for owner review: the
language surface). To be filled in.

## Context

Runtime and crypto code writes wrapping arithmetic as `rt.int_wrap(k,
a, b)` with a numeric operation code (0 add, 1 sub, 2 mul, 3 shift
left, 4 logical shift right) and the high word of a product as
`rt.u64_mul_hi`; Xo has no unsigned type.

## Decision

To be written: named operations (such as `wrap.add`, `u64.mulhi`, a
logical shift operator) replacing the numeric codes, and whether Xo
gets an unsigned type.
