# A codegen pattern that can never match passes forever

Status: Observed (2026-10-24)

## Symptom

`test/codegen/lists`' `sum` carried `// arm64: ... -"_xo_fault_index"`
(no bounds check in the loop) and passed with every xo, including ones
that did bounds check a loop: two new patterns written the same way for
`i % xs.len()` passed on the xo before the change they were meant to
prove.

## Cause

An absent pattern (`-"..."`) passes when the text never appears, and
`_xo_fault_index` never appears in a function's arm64 code: a bounds
check is `cmp` then `b.hs` to the function's local fault label
(`Lf1_fault3`), which calls the runtime later, and a release is
`bl Lxo_drop1`, a local stub. Same trap as a `go test -run` that matches
no test.

## Fix

Write an absent pattern for what the code holds (`-"b\.hs"`,
`-"bl _xo_h"`, both spellings of a call the emitter writes as tab, bl,
space, symbol), and check every new pattern against the xo before the
change: it must fail there (`XO_STD=BASE/std BASE/bin/xo dev codegen
--backend=arm64 test/codegen/DIR`) and pass after.

## Audit of every absent pattern (2026-10-26)

All 112 absent patterns in `test/codegen` (arm64 and llvm) were checked,
and 111 of them proven able to match: a copy of the test program with
each function's body written so the optimization no longer applies (a
variable multiplier, a index the loop does not bound, a 17 byte literal,
a direct call), or, for the `// func` sections, a std tree whose reader
functions hold the shape (a conditional pick, a division, a test of a
bit, a call), compiled with `XO_STD` pointing at it. Each pattern matches
in the variant and in no line of the real build. Ten could never match;
eight were corrected and two stay as guards:

- `arith` `packed`: `-"xo_fault_raise_shl" -"fault_shl"` named the middle
  of `_xo_x_fault_raise_shl`; both are `-"fault_raise_shl"` now.
- `lists` `cycle`: `-"bl Lxo_drop"` never matched a module function; a
  release there is `bl _xo_h<hash>`, and `Lxo_drop<n>` is a local label
  inside the runtime's own drop code. The guard is `-"bl _xo_h"`.
- `strs` `cut_len` and `enums` `is_mac`, `not_wasm`, `same`: the tab form
  `-"bl\t..."` (four patterns) matched no line; the emitter writes a
  space. The space forms stay.
- `arith` `u64_mod`: `-"smulh"` cannot come from a UInt64 remainder (the
  unsigned high multiply is `umulh`, a positive pattern there);
  `-"sdiv"` stays.
- `consttime` `mlkem.compress`' `-"udiv"` (arm64 and llvm) stays on
  purpose (lead, 2026-10-26): compress divides only Int today, but an
  unsigned divide is `udiv` in the IR (`rt.u64_divc` emits it for a
  UInt64 by a constant, which a std variant dividing two UInt64s shows),
  and udiv is as variable-time as sdiv, so the pattern guards a move to
  unsigned operands. The arm64 instruction stream cannot hold it today
  (the isel's only divide is sdiv), so there it is ahead of the code.

The variants are scratch (they lived under `/tmp/pat-audit` and are not
committed); to re-prove a pattern, write the variant the same way and run
the tool on both trees.
