# arm64 isel loaded whole fields that mir_dce then dropped

Status: Observed and fixed 2026-10-31 (decision 0448 stage 3).

## Symptom

The self build's emit held about 4.6 GB live halfway through (the heap
profile with e459813a2's fix), 1.5 GB of it the machine instruction
lists (`G.add`), while the largest functions kept only 1 to 2 percent of
what isel made: `ssa.Builder.mlog_ktype` 2,003,781 instructions after
isel and 22,865 after the first `mir_dce`; `ssa.Builder.trace_type`,
ten source lines, 885,177 and 8,326. The output was right; only emit's
memory and time paid.

## Cause

A `var b: Builder` method that reads `b.m.X` lowers `field(b, m)` and
then `field(that, X)`. `b` is held in memory and written later, so
`read_in_place` cannot read `m` in place, and `field_op` loaded all of
`m` (592 words) to give the second field read its one word. Every landing
pad's cleanup does this to release a field, about 740 pads in such a
function: 438,318 of `eq_thunk`'s 441,408 instructions were those loads
(cause C_LANDING, one `ldr` per word), dropped by the first `mir_dce`
since nothing read them.

## Fix

`field_op` loads through `load_read` (isel.xo): when every use of the
field read is itself a field read of it (`words_read`), only the words
those uses read are loaded; the rest are register 0, never read. A value
a source variable binds keeps the whole load (its location names every
word), and fields under 8 words are left as they were.

## How to find the next one

The probe that found it counted, per function, the instructions after
isel and after each pass (`G.function`), and tagged each landing `ldr`
with the SSA op being lowered; a pass that removes most of what isel made
points at isel making it. `XO_PROF=heap` with `XO_PROF_HEAP_AT=emit_half`
shows `G.add` and `load_words` when it comes back.
