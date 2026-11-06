# A darwin test that runs xo itself builds separately once std's variants are cached

Status: Observed (2026-11-06, darwin/arm64, xo-lang-4)

## Symptom

Eight darwin-only internal/arm64 debug tests (TestDebugVars/macho,
TestDebugLineDarwin, TestDebugLineDarwinCC/macho, TestDebugLineInlined,
TestDebugLoclistsBudget, TestDwarfReader/macho, TestDebugTypes/macho,
TestUnwind/fiber/macho) failed on a Mac whose back-end cache held std's
two variants, and all passed with an empty private
XO_BACKEND_CACHE_DIR. Messages: "has no __DWARF segment", ".debug_info
lacks DW_TAG_formal_parameter", "no __debug_loclists", `xo dev debug`
"print units" giving "<optimized out>".

## Cause

The tests run `xo build` themselves (not through xotool.Build), and xo
builds separately compiled by default when the cache holds std's
variants (decision 0388, sepenv.xo sepcomp_default; the test process's
XO_SEPCOMP_BG=0 only stops a miss from starting the background build).
A separately compiled darwin executable has a debug map and segment
__XODW instead of __DWARF (decision 0401), so a test reading the
executable's DWARF finds none. Which path a test took depended on what
earlier builds on that machine had cached, and darwin-only tests run
rarely, mostly on a fresh cache, so nobody saw it. One showed a real
bug: a Str literal's address in a location list, read through the
debug map, pointed at other bytes
(gotchas/dsymutil-loclist-address-function-offset.md, now left out of
separately compiled darwin builds). TestUnwind/fiber/macho printed a
std function's variable, which decision 0401 leaves to the dSYM by
design.

## Fix

A test of the whole program's link sets XO_SEPCOMP=0 in its build's
environment; the separately compiled path gets its own case
(TestDebugVars macho-sep, TestStdDebugMap, TestSepcompDebugInfo). To
see which path a build took, pass --phase-times: a separately compiled
build prints "cache std ... reused".
