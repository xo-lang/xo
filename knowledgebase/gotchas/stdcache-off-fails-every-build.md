# A backend cache that is off failed every native test build

Status: Observed (2026-11-08).

Symptom: under `XO_BACKEND_CACHE=off` (scripts/gate.sh exports it for
every step), every internal/arm64 test that builds a native program
failed in under a second with `xotool: std's variant conc=0 did not
build (exit status 1; XO_TEST_SEPCOMP=0 builds whole program instead):
cache std off (XO_BACKEND_CACHE=off): the whole program is built`
(TestBench, TestMapLogStress, TestSepcompGoldens, TestHeapGuard*,
TestRcpartFlat). The full tier and offload-gate-remote.sh never showed
it: they export `XO_TEST_SEPCOMP=0`, which skips the variants. A gate
script without that export did, and the failure first looked like the
change under test, then like 336733e1d (the watchdog's process group).

Cause: xotool's stdVariants prebuilds std's two variants with `xo dev
stdcache --conc=N` before the first native build, and since 87ce560bd
(2026-11-07) a variant that does not build is every later build's
error. With the cache off xo keeps no variant by design: std_variant
returns None and `xo dev stdcache` exits 1, while a build goes on whole
program.

Fix: stdVariants skips the variants when xo's cache is off
(internal/xotool build.go stdCacheOff: `XO_BACKEND_CACHE=off`, or
`XO_DUMP_FN` set, as compiler/xo bcache.xo backend_cache_mode reads
them); TestStdVariantsCacheOff. To tell this from a real variant failure:
the output after the colon is the "cache std off" line, not a compile
error.
