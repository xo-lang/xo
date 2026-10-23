# A loop with two back edges hoisted nothing

Observed and fixed 2026-10-22 (Xo Lang 1).

## Symptom

jsontext.str_end's word loop (`while true { if i + 8 <= n { ...
continue } ... i += 1 }`) made its five 64 bit byte masks and the
Bytes data address again every 8 bytes, on arm64 (mov and three movk
each) and amd64 (movabsq), where a plain `while i + 8 <= n` loop had
them in registers before the loop. json parse ran 1.36 times LLVM's
instructions on amd64 (bench/results/amd64-native-vs-llvm-2026-10-22.md).

## Cause

compiler/arm64 opt.xo dfs_loops returned one natural loop per back edge,
not per header. With two back edges (the `continue` and the loop's
end), each loop's body left the other latch out, so the header had two
predecessors from outside its "body" and no preheader: hoist_consts and
hoist_invariants skip a loop without one.

On x86 a second cause kept two of the masks in the loop: isel took an
arm64 logical immediate (0x2222..., 0x8080...) as the immediate operand
of and, orr, eor, and tst, which x86 has only for sign extended 32 bit
values, so x86_logic made it with a movabsq through r11 at each use.

## Fix

dfs_loops merges the bodies of a header's back edges into one loop (a
loops_test.xo case); on x86, isel's and/orr/eor and and_imm (tst) take
an immediate only when it fits 32 signed bits, so a wider one is a
register constant that hoist_consts moves.
