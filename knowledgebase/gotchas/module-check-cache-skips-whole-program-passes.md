# Module check cache: lazily checked bodies miss the whole-program passes

Status: Observed 2026-10-19 (decision 0220 addendum stage 2, step 2);
the cache is opt in (XO_CHK_CACHE=1) until fixed.

Symptom: the second build of internal/gen/testdata/golden/os_tasks with
the module check cache warm exited 0 and printed no error, where the
first build (and XO_FE_CACHE=off) exited 1 with "error: not a number: x"
from a task closure (`os.tasks.spawn(fn() ! { ... parse("x")? })`).
Found by the gate's llvm full tier (TestGenGoldensNative and
TestErrorTracesNative, os_tasks), not by verify or the identity runs.

Cause: a module skipped by the cache is checked lazily, its bodies only
when the SSA builder reaches them (check_lazy, as for the runtime's
modules, decision 0247). Checker.run's passes over all checked bodies
(solve_effects' fixed point over the inferred effects, report_discards,
check_realtime, flow_bodies, the never results, XO0402) run before the
builder, so they never see those bodies, and what they decide for them
(and for callers in other modules whose effects come from them) is
missing. The runtime's lazy modules avoid the features those passes
decide; program modules do not. Verify compares only the diagnostics,
and the identity runs build each program once, so neither sees it.

Fix (pending): store each pass's results for the module's bodies in the
entry and replay them, or check before those passes the bodies the
front end cache will not replay; test by building every golden twice
with XO_CHK_CACHE=1 and comparing outputs. Lazily checking a module is
only sound when no whole-program pass depends on its bodies.

XO_CHK_CACHE=auto (2026-10-20, opt in until verified on every golden,
the fixed point and a week without fallback warnings): the passes over
the checked bodies read nothing of another module's bodies, since only
a function that is not pub, not an interface's or std's and has no
`uses` gets inferred effects (decl.xo), and solve_effects reads only
those. So a skipped module matters only when one of its bodies is built
from source. auto skips a module only when its entry says every body of
it had a front end cache entry in the build that wrote the entry
(ModEntry.fe_complete: no skip reason, no generic instance, no closure
built apart); when a skipped module's body still reaches check_lazy (an
entry pruned or missing), or the checked bodies name another module's
inferred function (Checker.effect_reads_across_modules, the assumption
checked on every build), the driver warns, drops the build (storing
nothing in the front end cache) and checks those modules in full.
Tests: check_test.xo "only functions that are not pub ... have inferred
effects", internal/arm64 TestModuleCheckAuto (os_tasks and a golden
sample, warm and with the front end cache's entries removed, against
the cache-off binaries). Measured ceiling (XO_CHK_CACHE=1, edits of xo):
the check phase of a body-only edit 216 to 273 ms against 753 to 867 ms
without skipping, peak memory about 3.1 GB against 5.3 GB.

Generic functions (2026-10-20): after the closure relocation auto still
skipped 3 of xo's 21 modules on an edit (load 20 to 25, check 0.87 to
0.92 s): a generic function's instances are always built from source,
so every module declaring one had fe_complete false. Since then a module
entry lists the functions whose bodies its build built from source (a
generic function, a body the front end cache skipped, a contract's inner
body) and, in turn, the inferred functions of the module their effects
name (ModEntry.eager, check cache v4); a skipped module checks those
bodies before the whole-program passes, and only a closure built apart
still keeps a module from being skipped. A checked body naming a lazy
inferred function of its own module now also makes the build unsafe
(effect_reads_across_modules). Test: internal/arm64
TestModuleCheckAutoGeneric.

Which bodies were checked in time (Observed, 2026-10-21): the first
version let the builder build a skipped module's body from source
without falling back whenever `Checker.lazy_fns` said the body was no
longer waiting. That is also true of a body an earlier `check_lazy` call
checked during the build, after the whole-program passes, so
TestModuleCheckAuto's build with the front end cache's entries removed
stopped falling back. The builder now asks `Checker.eager_fns`: the
bodies check_module checked from the entry's eager list, before the
passes. Any other body of a skipped module built from source falls back.

Name records are indexed (Observed, 2026-10-21): replaying a skipped
module's entry removed the zone records of its eagerly checked bodies
before adding the entry's, but Checker.name_at holds indexes into
c.names, so every later record moved and the bodies' names resolved to
other objects (tz_table: `NAMES` read as `os`, 54 XO1201 errors on the
warm build; TestModuleCheckAuto). c.names is now only appended to: the
entry's zone records are added unless the eager check made the same one.
Test: internal/arm64 TestModuleCheckAutoZones.
