# A Str view's bytes are at raw.data

Observed 2026-10-25. Reading a Str's bytes in a native hot loop, the
question was whether the address helpers read a view's own bytes.

## Symptom

A per byte loop with `b[i]` pays a bounds check and an address add per
read, and it was not clear that `raw.data` reads the view rather than the
string it was cut from.

## Cause

`raw.data`, `raw.str_byte` and `raw.byte_at` read a view's bytes
correctly.

## Fix

An unchecked byte loop may use `raw.data` plus `raw.byte_at` after one
bound check before the loop. std/native/llrt/raw_test.xo shows it on
arm64 and llvm: len, bytes, a case mapping and str_byte over a view,
`byte_at(data(v), 0)` and `byte_at(data(v), 2)` of
`str_slice("abcdefghij", 7, 10)` (104 and 106), and a view of a view.
