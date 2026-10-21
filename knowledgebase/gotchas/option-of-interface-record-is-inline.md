# An Option of an interface record holds the whole record inline

Status: Observed (2026-10-21, Xo Publish, decision 0307)

## Symptom

A native capability value was far larger than its declaration suggests:
the native `Fs` with decision 0307's user field typed `Option[UserFs]`
was about 66 words (a function taking an Fs read it at offsets past 120
bytes, XO_DUMP_FN), Proc about 25, and arm64's entry failed with "main
takes too many words" once `Os` got such a field.

## Cause

compiler/ssa lowers an interface type to `xo.iface[I]`, a struct with one
closure (two words) per method plus its equality function and type id
(iface.xo iface_type), and an enum (Option) lays its payload out inline.
So `Option[I]` is about 2 x methods + 4 words, copied wherever the value
is copied.

## Fix

Hold the record behind a pointer: decision 0307's user field is
`xo.user[R]`, an enum of None and Some(Box[R]) (build.xo user_holder),
two words. A `List[R]` holder (one word, empty = rt.rl_empty) was tried
first; the arm64 inliner's verify then rejected the element read
(`v16 uses v15, which is not alive` after list.index, only after
inlining), so the enum of a Box is the shape to use.
