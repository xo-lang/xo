# A note of the Xo checker counted as an error

Status: Observed (2026-10-12, merge tier on main dbb8b05), fixed.

## Symptom

TestSelfhostArm64, TestSelfhostGen, TestSelfhostLLVM, and
TestSelfhostSSA reported "the Xo checker rejects it, the Go checker
does not" for bench/paper/v0.3/07-cache/{cache,itemsvc},
bench/paper/v0.3/08-notes-service/{domain,archtest}, and
internal/check/testdata/golden/effect_ceiling_vendor/vendor/example.com/{extra,tick}.
Both checkers printed the same single diagnostic: XO1315, `edition
2026` is the old name of `edition preview`.

## Cause

compiler/check's `Diag` has `warning: Bool` and, for XO1315,
`note_sev: Bool` (decision 0177). The note set `note_sev` only, and
every tool's `rejected` test (compiler/ssa, arm64, llvm, gen, xo main.xo:
`!d.warning || d.code == "XO0900"`) reads `warning` as "not an error".
A diagnostic of severity note therefore failed the check, where Go's
`diag.HasErrors` ignores notes. The runs only showed up once the xo.mod
lines of those modules were reported at all.

## Fix

A note sets `warning` too (load.xo report_edition_aliases); the field's
doc comment says `warning` means "not an error". A new severity added
to compiler/check must set `warning` unless it is an error.
