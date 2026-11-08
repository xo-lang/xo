# Lazy landings: cleanup's prune drops the unwind exit before rc_func needs it

Status: Observed (2026-11-07, decision 0562's prototype, Xo Lang 4)

## Symptom

With XO_LAZY_LANDINGS=1 every build failed in insert_rc: `fault: index -1
out of range for List of length 3 at ssa/ir.xo:946` (Func.jump) under
Builder.rc_run, before any golden ran.

## Cause

mark_landings makes a function's unwind exit up front (its builder,
finish_fn, can grow the module, so parallel rc jobs cannot make it), but
with the landings not yet made no edge reaches the exit. cleanup's
prune_func keeps only the blocks reachable from the entry, so it removed
the exit and set unwind_exit to -1; land_func then jumped to block -1.
Func.shell (used by renumber) copies a Func's header field by field, so a
new Func field is also lost there unless it is added.

## Fix

Func.lazy_lands (set by mark_landings, cleared by land_func) makes the
exit a root of prune_func, and Func.shell and rccache's encoding carry it.
A new Func field that must survive cleanup goes into shell() and, when
the caches keep bodies, into rccache's encode and decode.
