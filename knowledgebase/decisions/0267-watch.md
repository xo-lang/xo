# 0267: xo run --watch and xo test --watch

Status: Accepted (owner, 2026-10-18, through the lead); stub, to be
written and implemented by Xo Lang 3 after decisions 0261 stage (a) and
0262

## Scope agreed

- `xo run --watch`: rebuild on file changes; restart only when the built
  program changed (test-only edits never restart; a byte-identical
  binary does not restart); servers get a graceful handoff (SIGTERM,
  wait for exit with a deadline, then start).
- `xo test --watch`: shares the machinery; reruns only the tests that
  reach a changed function (per-function hashes, the run cache's input
  list) plus tests in changed _test.xo files, with a key to run all.
- Both: `--json` events per cycle (changed files, diagnostics, rebuilt or
  skipped, restarted or not, the new pid; for tests passed, failed,
  skipped as unaffected, seeds of failures) and a way to query the last
  cycle's result; the backend function cache keeps cycles fast.
- When it ships, spec/core.md 9's "Where tests live" note says --watch
  needs no file-name exclusions, since it compares the built program.
