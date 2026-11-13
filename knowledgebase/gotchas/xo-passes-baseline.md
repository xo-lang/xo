# Measure a pass against an XO_PASSES baseline, not an unset one

Status: Observed (2026-11-13, Xo Lang 3, measuring decision 0624's deadw).

## Symptom

`XO_PASSES=+deadw` made golden enum_dead_words' `__text` 26.6 KB larger
(351,232 to 377,840 bytes, darwin/arm64, XO_INLINE_STRICT=1), with 422 more
functions out of line (`run`, std/mlog and map helpers), for a pass that
only skips stores. `XO_PASSES=+shareret`, a pass already on by default,
gave 377,972: the same growth with no pass changed.

## Cause

Any `XO_PASSES` value, `+` or `-`, takes the build off the backend
caches (compiler/xo main.xo, `passes_changed` before fn_cache_load) and
off the std facts (bcache.xo rc_cached), since their keys do not say
which passes ran. That path lays out and inlines differently, so the
build differs before the pass does anything.

## Fix

Compare a pass against a baseline with `XO_PASSES` set to a no-op, such
as `+shareret` (any pass on by default): enum_dead_words is 377,972 there
and 377,840 with `+deadw` (-132 bytes). A size or speed claim for a pass
names both settings. An unset `XO_PASSES` against `+PASS` measures the
cache path, not the pass.

Scope: only builds that use those caches. A whole program build
(`XO_SEPCOMP=0`, as the gcp size samples) is the same with and without
`XO_PASSES`: BigQuery, Calendar and aws presign were byte-identical with
`XO_PASSES` unset and `+reltail` (2026-11-13), so shareret's and deadw's
sample sizes, measured that way, stand.
