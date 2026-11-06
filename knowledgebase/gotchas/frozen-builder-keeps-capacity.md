# A frozen StrBuilder keeps its doubling slack

Status: Observed (2026-11-06)

## Symptom

xo's self build for linux/amd64 mapped about 6.9 GB at its peak (max RSS
5.1 GB), and the peak rose 0.9 GB in the hollow phase. A heap snapshot at
the end of emit (`XO_PROF=heap XO_PROF_HEAP_AT="past phase emit"`) held
1.17 GB of assembly text, allocated in arm64.drop_weak_in; one at the end
of hollow held 1.83 GB, allocated in hollow_rewrite, for texts no longer
than the ones it replaced.

## Cause

`StrBuilder.write` appends in place and grows the string by doubling
(std/native/llrt/obj.xo reserve_slow), and `freeze` is the builder itself
(compiler/ssa/call.xo lowers it to its receiver): nothing trims the
capacity, so a frozen text keeps up to twice its length. A text that
lives long (the program's assembly parts, from emit to the link) carries
that slack the whole time, and a rewrite of all of them holds the old and
the new slack at once.

## Fix

Where a large builder's text outlives the function that made it, return
an exact copy: `"" + sb.freeze()` (concat_o copies into a new string of
exactly the length, and the builder is freed at its last use). Done in
hollow_rewrite and drop_weak_in. A general trim in freeze is a separate
item, parked (lead, 2026-11-06; branch l4-freezetrim): it works for
big blocks (a 2.1 MB builder held 4,193,920 bytes, its trimmed copy
2,100,001) but gains nothing below 256 KiB, where the allocator's size
classes are powers of two (a 140,001 byte text and its builder both held
262,120), and on xo's stage 2 self build it added nothing over these two
copies (amd64 peak 6.48 / 6.21 GB against 6.28 / 6.33), which were the
only big long-lived texts.
