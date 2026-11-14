# Kept assembly overstates the program's instructions

Status: Observed (Xo Lang 1, 2026-11-14).

## Symptom

Instruction counts taken from the `.s` files an `XO_KEEP_ASM=1` build leaves (every `.s` under
the build directory concatenated, each indented non-directive line counted) said xo's self build
had 18.8 M instructions, 1.85 M of them in release ladders and 1.38 M (7.4%) in repeated ladders.
The linked binary had 9.32 M (`size -m`, `__text` / 4; `otool -tV` lines), 0.53 M in ladders and
0.50 M (5.3%) in repeats, so decision 0634's first estimate (about 7% of xo) became 4.9% measured.

## Cause

The kept assembly holds every function the backend emitted, including the ones the hollow pass
(decision 0371) reduces to one trap and the ones the link drops, and the counts include them. The
ratios inside one kind of code (one function, one program's reached code) are close; the whole
program's are not.

## Fix

Size claims come from the linked binary: `size -m BIN` for `__text`, and `otool -tV BIN` (or
`objdump -d`) for instruction mixes, as decision 0634's Results do
(~/xo-lead/notes/lang1/ladders-bin.py reads otool's output). Use the kept assembly to see what a
function looks like, not to count the program.
