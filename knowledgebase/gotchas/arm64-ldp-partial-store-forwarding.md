# A load pair over bytes a single store wrote in part stalls (Apple cores)

Status: Observed (2026-10-26, nbody), fixed for d register pairs; the x register case is open.

## Symptom

bench/perf/llvm nbody ran 1.43 times Go's wall (about 970 M cycles against Go's 664 M) while
retiring fewer instructions (2.565 G against 3.015 G): IPC 2.7 against 4.5. The gap was the same
with the sqrt and the division removed (13.2 against 11.3 cycles a pair), so not the divider.

## Cause

The inner loop updates each j body's vx, vy and vz with single stores (`str d`, or one `str q`
for the SLP pair), and the next outer iteration loads the body's fields as the i body with
`ldp d28, d23, [x11, #40]` (z, vx) and `ldp d22, d24, [x11, #56]` (vy, vz). A 16 byte load that
spans bytes one earlier store wrote in part cannot be forwarded from the store buffer: the core
waits for the store to commit, about 10 to 15 cycles each, four or five times an advance (the 57
cycles a advance that Xo lost). Go loads every field with its own 8 byte `FMOVD`, which always
forwards. Proof: a hand edit that split those two pairs into single `ldr`s took the cycles from
1,001 M to 711 M (instructions +2%); not the alignment (padding Body to 64 bytes changed nothing),
not cluster_mem (off: no change), not SLP (off: worse), not FMA (the spec decides).

## Fix and rule

lsr.xo `pair_mem_l` and its items port lines.xo `pair_mem_ops` do not pair two loads of d
registers when some store in the function (`str`, `stur` or `stp` of x, d, w, s or q registers
through a non sp base) writes part of the same 16 bytes without being exactly them
(`partial_store`). Any base register counts: the store and the load usually reach the bodies
through different registers. sp pairs (the frame's saves and restores) are not touched.
nbody: cycles 962 M to 722 M (0.75), instructions +1.9%, the same output.

## Open

The x register pairs are left as they were (a first version that covered them broke the existing
16 byte copy through two bases, `ldr x10, [x11]; ldr x13, [x11, #8]; str x10, [x9]; str x13, [x9,
#8]`, which pairs and is fine). Whether an x pair over an 8 byte store stalls the same way in a
real program (a List's header words, a struct of Ints updated field by field) is not measured:
look for it with the hand edit harness (`macho asm` and `macho link` on the generated `.s`, an
unedited control first) before widening the rule. The hazard is the same on every Apple core;
other targets are unmeasured.

### x pairs on a quiet machine (2026-10-26, load 1.7 at start, middle and end)

The same comparison (every x `ldp` split, 9 interleaved runs of 16 json, bytessearch and sort
workloads, min and median cycles of the split build over the control) gave 0.97 to 1.07 on the
minimum and 0.97 to 1.02 on the median, instructions +0 to +2%: no stall. The x register pair is
not a measured hazard on these programs; the d register rule stays as it is. The nbody wall
(`xo bench --ab`, 15 rounds, same window): 228.2 ms to 171.1 ms (0.75, 0.747 to 0.756), instructions
+1.9%.

### x pairs on real code (2026-10-26, first look)

Splitting EVERY x `ldp` of a program into two `ldr`s (3,062 of 4,041 in the text bench, 2,059 in
json, 1,609 in bytessearch, 940 in sort; script `splitx.py` in the lead's notes, linked with
`macho asm` and `macho link`) and comparing cycles with the unedited control on json, bytessearch
and sort workloads gave ratios between 0.76 and 1.19 with the median and the minimum of nine
interleaved runs disagreeing (cut: min 1.19, median 0.91; rfind_any: min 0.76, median 0.99) at a
load average of 28 to 34: noise, no reproducible stall, instructions +0 to +2%. Not conclusive
below about 10%: repeat on a quiet machine, per workload, before widening the rule. A static scan
for an x `ldp` and a store through any base that covers it in part finds hundreds of functions
(CRUD: 262), which only says the pattern exists, not that it is hot.

## Measuring

Instructions retired do not show it; use cycles (`/usr/bin/time -l`: "cycles elapsed" next to
"instructions retired"), interleaved with a control, on a machine whose load is known. The harness:
`xo build --asm=prog.s`, `macho asm -o prog.o prog.s`, `macho link -o prog prog.o` (compiler/macho
built with xo), after checking that the unedited `.s` reproduces the xo binary's instructions and
cycles.

## Known gap in the guard (DS-2's review, 2026-10-26)

The guard (`partial_store` in compiler/arm64 lsr.xo and lines.xo) compares the offsets and sizes of
stores and loads through a base, so it can only over suppress a pair, never form one it should not
(the aliasing direction is safe). The gap is the other way, performance only: lsr.xo:1199 and
lines.xo:331 need an explicit immediate offset, so a store written `str d0, [x1]` (no offset), a pre
or post index store and a register offset store are invisible to it, and an ldp over bytes such a store
wrote still forms and can stall. Fix only if a benchmark shows it (none does today).
