# Parallel `xo test --std-module` runs overwrote each other's binary

Status: Observed (2026-10-20)

Symptom: `xo test --std-module=M` for several std modules at once, from
the same directory, failed at random: tests reported "no test with this
index" or a property failed on case 1. Each module passed alone. Seen
when TestFrontCacheAcrossPrograms first ran the std modules' tests eight
at a time; it failed the same way with XO_BACKEND_CACHE=off and on
main's xo, so it was not the front end cache.

Cause: the native build directory (compiler/xo cache.xo build_dir) is
keyed by the program's directories, the mode and variants. A
`--std-module` test names no directory, so every std module's test
binary in one working directory went to the same build directory and
name, and one run executed another module's binary.

Fix: build_native passes `std-module=M` as a build directory variant.
TestFrontCacheAcrossPrograms runs the std modules' tests in parallel and
covers it.
