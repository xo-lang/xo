# 0437: no silent success in the tools

Status: Accepted (owner, 2026-10-30; was Proposed (xo-lang-4, 2026-10-30; the owner's queue item 1 from the agent survey of 2026-10-30, ~/xo-lead/notes/lead/survey-2026-10-30/SUMMARY.md))

## Context

The agent survey of 2026-10-30 ranked "silent success" first among the
things that cost agents attempts: a command that did nothing looked like
it passed.

- `xo test` printed `ok: 1 passed, 0 failed, 1 skipped` for a test that
  declares `uses (fs)`; why it was skipped (it needs `--real`) showed only
  with -v (Lang 4).
- `xo test --std-module=base64` ran std/base64 (the Go backend's module,
  no tests), not native/base64, and passed with nothing run; a misspelt
  module did the same; `xo test` on a path in std tested nothing as a
  program (Lang 4, Lang 6).
- `XO_DUMP_FN=f xo build` a second time was served by the run cache
  (decision 0360) and wrote no dump (Lang 1).
- `xo query` outside a module printed an empty answer (Lang 2).
- `go test ./internal/arm64 -run TestGolden` (the LLVM package's name)
  printed "no tests to run" and ok (Lang 2; the memory note
  golden-test-names).

## Decision

A tool command that did nothing reports it, or fails:

1. `xo test`'s summary line names its skips without -v: the `uses` tests
   in one entry with their effects, then each other reason
   (`ok: 1 passed, 0 failed, 8 skipped (8 uses clock, env, fs, fs.read,
   net, proc, stdio: run with --real)`), on the native harness (compiler/xo test.xo skip_summary)
   and the Go backend's (std/rt testing.go skipSummary) alike.
2. `--std-module=NAME`: a bare name is the one of native/NAME and
   std/NAME that has tests; a name with no module, or a module with no
   tests, exits 2 with the nearest tested modules (check.nearest, made
   public); the flag implies `--fail-empty`. `xo test PATH` with PATH in
   std exits 2 naming the `--std-module` to use.
3. The run cache (decision 0360) is off for a build or test run that sets
   a variable asking for what only compiling writes: XO_DUMP_FN,
   XO_INLINE_REPORT, XO_CACHE_STATS, XO_FE_STATS, XO_PARSE_STATS, XO_PROF,
   XO_PROF_SELF (testcache.xo BUILD_DEBUG_ENV).
4. `xo query` on a directory with no xo.mod above it and no .xo files
   exits 2: "not inside a module ...; pass --dir=DIR".
5. In the packages with goldens (internal/arm64, internal/llvm,
   internal/gen, cmd/xo), a `go test -run` whose first element names no
   test or example exits 1 (testtmp.RequireRunMatch in TestMain); `^$`,
   -bench, -fuzz and -list runs are left alone.

The diagnostics half of the survey's findings (did-you-mean for foreign
idioms) is Lang 2's, not this decision.

## Consequences

- Tests: cmd/xo TestNoGCTests/real (the summary on both runners),
  TestXoTestStdModuleNames, TestWholeBuildDebugEnv, TestQueryOutsideModule;
  internal/testtmp TestRequireRunMatch. Each fails before its change.
- A run with one of the debug variables set always compiles: slower, but
  what was asked for is written.
- A script that relied on `--std-module=NAME` passing with nothing run
  now fails, which is the point.
- RequireRunMatch reads testing.M's unexported test list by reflection;
  if Go changes that layout it finds no tests and does nothing, rather
  than failing every run.

## Alternatives

- Print "xo: XO_DUMP_FN ignored: the run cache served this build" instead
  of turning the cache off: still a wasted attempt; the agent reruns with
  XO_WHOLE_BUILD=off anyway.
- A wrapper script for `go test` instead of TestMain: agents and scripts
  call `go test` directly, so a wrapper would be skipped exactly when it
  matters.
