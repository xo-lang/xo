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
