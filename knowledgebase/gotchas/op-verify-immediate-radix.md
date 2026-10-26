# XO_OP_VERIFY fails on an immediate written in hex

Status: Observed (2026-10-26, DS-1).

## Symptom

`XO_OP_VERIFY=1 xo build --backend=arm64 .` fails on an ordinary
instruction:

    XO_OP_VERIFY: obj.rs_append_bytes: movi v31.16b, #0x19 => movi v31.16b, #25

even with no change of your own: any program that pulls in the case
fold (here from `std/native/llrt/obj.xo`, as `bench/perf/llvm/xo/nbody`
does) stops at the first such line.

## Cause

`compiler/arm64/calls.xo` writes six immediates in hex (the `movi` of
the fold lane, and the `and` masks `#0x1111111111111111`,
`#0x0303030303030303`, `#0x000f000f000f000f`, `#0x000000ff000000ff`
and `#0xffffff`), and `print_op` writes every immediate in decimal.
`op_round_trip` compared the text's print with the line as strings, so
one word spelled two ways read as a difference and the check never got
past the first one.

## Fix

`op_round_trip` (`compiler/macho/op.xo`) now compares operands: two
immediates that `imm` (enc.xo) parses to the same value are the same
operand, every other difference (a register, a memory operand, another
value) is still a mismatch. Both spellings assemble to one word, which
`macho_test.xo`'s ENCODINGS asserts for `movi v31.16b, #0x19` against
`#25` and for `and x9, x9, #0xffffff` against `#16777215` (clang 21).

Sweep after a change here: `XO_OP_VERIFY=1 XO_BACKEND_CACHE=off xo
build --backend=arm64` of `bench/perf/twins/hello/xo`,
`internal/llvm/testdata/golden/json_text`, `.../map_branch` and
`bench/perf/llvm/xo/nbody`, plus `xo test compiler/macho`.
