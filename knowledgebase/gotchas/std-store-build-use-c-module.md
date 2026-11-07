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

Guard (lead, 2026-11-06): scripts/gate.sh runs internal/arm64
TestStdStoreBuilds for any change to a std module's source (std/*.xo,
std/native/*.xo or a std/native/MOD/ directory), so a module that breaks
the store build fails its own gate instead of the next main check.

Second place (2026-11-06, Xo Lang 3 found it on a fresh linux/arm64
checkout): `xo dev edges` preloads every std implementation file
(compiler/cli/servers edges.xo impl_modules), and a `use c` module there
needs its C binding (`.xo/cbind`), which only a build with a C compiler
writes; a checkout that had built the module passed, a fresh one failed
cmd/xo TestDevEdges. The spelling scan of `xo def` and `xo refs`
(compiler/cli/semantic query.xo preloads_wide, which adds every std file
that spells the name) did the same: cmd/xo TestAgentRefsSamePath failed in
the Mac hourly worktree (`xo refs prelude.Str.to_lower`). Both now leave
such a module out through check.uses_c, as the std store build does.
Anything else that loads every std module (a sweep, a coverage tool) uses
check.uses_c too, and its test runs on a tree without `std/native/.xo`.

Third place (2026-11-06, the lead asked for the repository root): `xo def
sqlite.version` and `xo doc sqlite.driver` failed with "the check stopped
before resolving names (use c without the binding ...)" from the repo root
and from any module, since naming a std module loads its implementation:
preloads_for adds native/NAME by the name's first segment, load_impl loads
the implementation of every std module loaded, and load_go_compiled loads
std/go_compiled.txt's modules (sqlite is listed). A declarations only
check (Checker.decls_only: def, doc, symbols) now leaves out an
implementation with `use c` (Checker.skip_impl), and preloads_for no
longer names one; std/NAME.xo answers. cmd/xo TestAgentRootStdUseC runs
them from the repo root (it shows the bug only on a tree without the
binding, as a fresh checkout is).

Fourth place (2026-11-06, the abac4a7da main check, cmd/xo
TestAgentRuntimeRefs): `xo refs --dir=std/native pool.park_wait` checks
std/native as one directory module, so sqlite.xo's `use c` was in it, and
sqlite_test.xo's `use std/sqlite` loaded native/sqlite with its body (refs
is not declarations only). Two changes: load_dir leaves a `use c` file out
of std's own native directory, and an agent's or editor's view
(Checker.view_only, set by compiler/cli/servers lsp.xo check_view_walk)
no longer stops on a std module's missing binding: that module's C names
stay unresolved, and the lookup answers from the rest. A build still
stops. The whole cmd/xo TestAgent* group, TestDevEdges and TestSymbols*
are the gate for any change here, run on a tree without std/native/.xo.
