# A builder fact read from the checker's names is missing for a cached module

Status: Observed (2026-10-22), fixed the same day.

## Symptom

internal/arm64 TestModuleCheckAuto (generics, http_cookies,
http_router_paths, lazy_const, map_log_iter, xml_basic) and
TestModuleCheckAutoGeneric failed: the warm `XO_CHK_CACHE=auto` build
and the build after a fallback were not the cache-off build's binary.
The program ran the same; the warm binary was about 1.5 KB larger, with
drops of std's user file system (`UserStdio`'s box) and of closure
types the cold build did not have.

## Cause

129b19af7 (decision 0307 follow-up, 2026-10-21) folds main's own Os to
the runtime's capabilities when no identifier names main
(ssa build.xo main_unnamed), read from the checker's names. A module
whose check comes from the module check cache is not checked, so it
records no names, and main_unnamed then answered "named": the warm
build kept the user capability dispatch (and its drops) that the cold
build folded away. The option is opt in, so default builds were never
affected; the tests are not in any per-push gate, and the bisect (with
era-correct xobuild seeds) took 8 steps over 321 commits.

## Fix

The module check entry records the answer (ModEntry.main_unnamed,
check cache v6: the module's `main` named only by its declaration,
Checker.names_only_decl), and main_unnamed takes it for a skipped main
module (mod_cached_main_unnamed). Any new builder decision that reads
the checker's per-body results (names, var uses, effects) of the main
module has the same trap: record it in ModEntry, or it differs warm.
