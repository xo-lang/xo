# A front end cache entry counted the made up variables of helpers it made first

Status: Observed (2026-11-15, xo-lang-1, task #44; fixed in front cache v19).

## Symptom

internal/arm64 TestChainKeysSharedCache failed on main (530cddfba, host 3
and the Mac): the verify build of golden http_header_fields stopped with
"XO_BACKEND_CACHE=verify: 1 function bodies' SSA differs from their front
end cache entries, the first http.header_get". Replaying the cached entry
gave the same function, value for value (fe_func_diff "other"). With
XO_FE_SPEC=0 the same sequence passed 3 of 3; with the speculative front
end on it failed 3 of 3 on the Mac (about 1 in 4 runs with an xo built
with debug output, whose timing differs).

## Cause

An entry records how far the body advanced the builder's made up variable
counter (Builder.nvar, Builder.local), and its replay advances it as much.
It was the counter's change across the body's window. A body that is the
first in the build to need a generic helper builds that helper inside its
window (header_get's Map loop made xo.maph.logget[Map[Str, Str]]), and the
helper's own made up variable counted as the body's: 5 in one build, 4 in
a build where an earlier body (or a speculative job ahead of it) had made
the helper. Which body comes first depends on the build's order, so the
same body's entry differed between programs sharing a cache. The count
changes no output (the ids only key one function's construction), so
only verify saw it.

## Fix

fe_begin(fi) names the function being recorded and Builder.local counts
the variables made while that function's state is current
(fe_nvar_own); the entry records that count (FE_CACHE_VERSION v19, so no
entry with the old count is compared). Rule: anything an entry records
must be what the body itself did, not a difference in builder state
across the body's window, since helpers and types are made in the window
of whichever body needs them first (the same lesson as
fe-cache-one-way-flags.md and tail_sans_mods's module list).

The one-way flag bits followed the same order (one run in four after the
nvar fix): lowering a struct with a task handle field (Atomic, Mutex,
Chan, ...) sets need_conc in named_type, but only where the struct is
made, so http.respond's entry carried the bit in a build where it made
such a struct and not in one where another body had. fe_entry now sets
the bit too when the body read a type made before it whose shape holds a
task handle (fe_reads_conc), so the entry is the same either way and a
replay sets need_conc as the build from source would have.

Test: internal/arm64 TestChainKeysSharedCache (failed 3 of 3 sequence runs
on the Mac before the fix).
