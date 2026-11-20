# arm64 regalloc: a register's occupancy must stay disjoint

Status: Observed 2026-11-18 (task #381, call_saves).

## Symptom

With call_saves on, bench/perf/brotli's `enc11` output was 13,800 bytes
where it should be 13,312 (md5 32441cc7696bc3713761f2e7a2b01274). It
still decoded, so only the compressed size showed it. Every one of the
30 save calls in `brotli.histogram_combine` had to be on: turning any
one of them off fixed the output. Static checks of the assembly found
no register left live across a `bl` without a save, so the clue came
from a dynamic trace. The 7th `histogram_combine` call returned 1 where
the good build returned 2, and its first `hs_merge` merged (45, 46)
instead of (15, 46).

## Cause

`best_idx2` (v147, ranges [278, 987] and [1212, 1223]) was saved around
four calls in x14. A list address hoisted into the shift loop (v175,
[542, 611]) was then given x14 as well. A save-path value is inserted
into `occ[r]` on top of the save calls' points, which are already
there: `occ[14]` read [278, 987], [285, 285], [542, 611], with ranges
nested inside one another. `overlaps_sorted` binary-searches by range
end, so it needs disjoint ranges. On this list it found [285, 285],
which ends before 542, and missed [278, 987].

## Fix

`insert_merged` (regalloc.xo) merges `occ[r]` after a save-path insert,
so the list stays disjoint. A test is in
compiler/arm64/callsaves_test.xo. Any new path that inserts a range
into `occ` over ranges already there must merge too, or show that the
ranges cannot overlap.

The same work found a second limit. emit.xo `scratch_reg` reloads an
instruction's third spilled operand into a caller-saved register that
is free at that instruction. Once saved values filled every
caller-saved register, `xo.mlog.vals`'s `madd` had none left, and xo
did not build itself. `save_regs` keeps the last caller-saved register
out of the save path.

2026-11-20 (task #465): one register was not enough either. A template
that writes registers of its own or names x16 or x17 (windows/arm64's
reference count decrement, `ldxr x17 ... stxr w15 ...`, which has no LSE
form there) reloads even its first spilled operand into a caller-saved
register, and it writes x15, the one `save_regs` leaves. With saved
values in the others, amqp.st_begin failed with "no free register for a
spilled operand", so `xo dev stdobj --target=windows/arm64` and
TestSepcompLinksWindows/arm64 were red from fc8c0a611 on. The save path
now counts the saved values over each such instruction
(`hungry_points`) and keeps HUNGRY_FREE (2) of `save_regs`'s registers
clear of them there; a value that would be one too many spills as it did
before call_saves. A new template with clobbers needs nothing more; one
that needs more than two registers beyond its clobbers raises
HUNGRY_FREE.
