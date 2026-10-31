# A multi-line arm64 template line must not start with a tab

Status: Observed (2026-10-31, Xo Lang 1, rc_thunk)

## Symptom

A K_PLAIN template `"mov x16, $u0\n\tbl SYM"` (a call to a thunk that
clobbers x16 and x17) built programs that computed wrong results
(sort_str: 16 mismatches) or crashed (the http goldens, the self-built
xo), only with the pass on.

## Cause

The emitter puts one tab before every line of a template, so the second
line came out as `\t\tbl SYM`. The text passes that run on the emitted
function (compiler/arm64 lsr.xo reload_reuse_l and writes_scratch) take
a line's opcode as what follows its first tab: `\tbl` is no call to
them, so they kept believing x17 held a spill slot across the thunk and
dropped the next `ldr x17, [sp, #N]`.

## Fix

Write a template's later lines without a leading tab (`"mov x16,
$u0\nbl SYM"`), as isel's `fcmp ...\nfccmp ...` does. atomic_inc and
atomic_dec still return `\n\t` texts; they are used inside helper bodies
and a cold block, where no x16/x17 reload follows, but a new K_PLAIN
user of them should strip the tabs.
