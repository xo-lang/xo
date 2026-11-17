# 0701: xo test --since=BASE, and cached --std-module runs

Status: Accepted (owner, 2026-11-17, through the lead: task #250, "run only the affected tests"); written and implemented by Xo Lang 2. Amends 0267 and 0439.

## Context

Agents gate every push with test runs that mostly repeat what passed
before: a std push runs its module's tests and others' whose code it did
not touch. xo already had two parts of a fix: `xo test --watch` (0267)
selects the tests a change reaches, but only inside a watch loop, and the
result cache (0439) replays a passed run, but left every `--std-module`
run out (its stage 1), so std's own module tests always ran.

## Decision

1. `xo test --since=REV [DIR]` and `xo test --std-module=M --since=REV`
   run once, only the tests a change since the git revision REV reaches
   (compiler/xo/since.xo). The change is `git diff --name-only REV` plus
   untracked files, below the module root (paths through git's
   `--show-prefix`, so a root reached through a symbolic link, as macOS's
   /var, still matches). The rule is 0267's, by names:
   - the files of what is tested: every watched .xo file below the
     module root, or with --std-module M's own (M.xo, its directory,
     native/M.xo, native/M_native.xo and the tests among them);
   - a changed source (.xo, .c, .h, .s, xo.mod, xo.lock, xo.work)
     outside them runs every test; a change to no source (docs) runs
     none ("ok: 0 passed");
   - otherwise a declaration whose text changed, appeared or went is
     changed, a declaration whose text names a changed one is changed
     too (to a fixed point), and a test runs when it is new or edited or
     names a changed declaration.
   The selection goes to a child `xo test` in XO_TEST_ONLY, as --watch's
   cycles do, so the run cache's binary is reused. Names, not the
   checker's calls: the set errs toward running, never misses a caller
   that names the changed function, and a passing --since run is as good
   as a full run for the change. REV that is not a commit, or a directory
   outside git, is exit 2. A bare `--since` compares with the merge base
   of HEAD and origin/main (main without an origin). With `--json` the
   first line is the selection: `{"since": REV, "changed": [...],
   "selected": [{"name", "why"}] or null, "all": why every test runs}`,
   a why being "new or edited", "names F, changed" or "names F, which
   names G"; the run's own JSON lines follow.
2. 0439's stage 2: a `--std-module=M` run is stored and replayed like a
   program's unless M is under std/native/llrt (or is native/rt) or a
   test file of M has a `use native/...` line (it reaches the machine
   past the TestOs fakes). The other 0439 rules stand: a `uses` test does
   not run without --real, a binary linking a binding writes no pure
   marker, and the key hashes the run cache entry's recorded inputs, so
   an edit to any std file the build read runs the tests again.

## Consequences

- Measured on 5 std pushes (bench/results/test-since-2026-11-17.md): an
  unchanged module replays in 0.05 s; within a changed module --since
  kept most tests (the pushes edited their tests too) and its git and
  parse work (0.2 to 1.3 s) cost more than these modules' 0.1 s runs.
  The saving is in modules a push does not reach: --since selects none
  and the cache replays them. A gate step choosing the reached std
  modules is the next step (not in this decision).
- `xo test --since` needs git on PATH; --watch does not.
- Tests: cmd/xo TestXoTestSince (no change, a docs change, a changed
  function and its caller, a new test, xo.mod, a bad revision),
  TestXoTestStdModuleResultCache (replay, an edit to the module's source
  runs again, an llrt module never replays, --std-module with --since
  takes no directory).

## Alternatives

- The checker's call graph instead of names: exact, but a std module's
  check costs more than its tests; names over-select safely.
- Compare BASE's checked definitions (0267's defhash) by building a copy
  of the tree at BASE: precise for a program, but a second check per run.
- Leave std modules uncached: every gate keeps running tests of modules
  the push did not change.
