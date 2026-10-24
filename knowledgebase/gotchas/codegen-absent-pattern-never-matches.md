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
`-"bl Lxo_drop"`), and check every new pattern against the xo before the
change: it must fail there (`XO_STD=BASE/std BASE/bin/xo dev codegen
--backend=arm64 test/codegen/DIR`) and pass after.
