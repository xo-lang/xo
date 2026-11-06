# A std module with `use c` broke std's store build

Status: Observed (2026-11-06, main 7b1cbe8da, Xo Lang 2).

**Symptom.** Every native build warned "separate compilation is off: std's
store build failed" and built the whole program. TestDevScaffolds,
TestLogAttribution, TestCLIRunNoWalk, TestStdStoreBuilds, TestUnwind's sep
cases and the RSS guard failed. The failure is cached in the shared backend
cache for an hour, so every agent's builds slowed at once. Repro:
`XO_BACKEND_CACHE_DIR=$C/b xo dev stdobj --target=darwin/arm64 --conc=0
--facts=$C/std.stdf $C/obj` fails with `Undefined symbols: "_xo_entry"`
from `/usr/bin/cc`.

**Cause.** std's store is built from an empty program with every std module
as a root (XO_STD_ROOTS, decision 0388 1b). std/sqlite (decision 0534) is the
first std module whose native file has `use c`. A program with a `use c`
module links with cc, not Xo's linker (main.xo default_ld). On the cc path
the store build links its objects instead of writing them as std's store
(that branch is on the Xo assembler's path only), and the empty program has
no entry. The sqlite push's gate passed because a failed store build only
warns, and the goldens built whole program.

**Fix.** check_dirs leaves out of the std roots any std module whose
`std/native/NAME.xo` has a `use c` line (Checker.std_uses_c, as stdgen's
usesC does for the Go backend). A program that imports such a module builds
it as its own part. When adding a std module with C sources, run
`xo dev stdobj` (or TestStdStoreBuilds) in the gate: a build that falls
back to whole program still passes most tests.
