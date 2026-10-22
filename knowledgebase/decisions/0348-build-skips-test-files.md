# 0348: `xo build` and `xo run` do not read `_test.xo` files

Status: Accepted (owner, 2026-10-22).

## Context

`xo build` and `xo run` load every `.xo` file of a module, `_test.xo`
files included, and check them before building. A type error in
`main_test.xo` fails `xo build` (confirmed with 0.25.2), a helper
declared in a test file is visible to the program while it builds, and
a `use` that only a test file has makes the build fetch that module or
bind that Go package or C header. No decision chose this; it fell out of
the loader listing a directory's `.xo` files. Test blocks themselves are
already compiled only by `xo test` (core 9.2), and decision 0262 put the
repository's own tests in `_test.xo` files so that build caches keyed on
sources skip them by name, which the build itself did not do.

## Decision

1. `xo build` and `xo run` (with or without `--watch`) neither parse nor
   check `_test.xo` files, and print nothing about them. A test file
   costs a build nothing, and a broken one does not stop it.
2. `xo test`, `xo check`, `xo vet`, `xo fix`, `xo fmt`, the language
   server, the agent tools (`xo def`, `xo refs`, `xo type`, `xo doc`,
   `xo symbols`, `xo query`, `xo edit`, `xo mcp`) and `xo serve` keep
   loading them as today.
3. Items declared only in a `_test.xo` file (helpers, types, consts) do
   not exist for a build: production code that names one is `XO0601`
   in `xo build`, as it should have been.
4. A `use` that appears only in `_test.xo` files is not fetched, bound
   (`use go`, `use c`) or linked by a build.
5. The build's caches stop seeing test files: the module check cache's
   entry, the parse list, and the front end cache are all keyed on the
   files the check parsed, which are now the non-test files, so an edit
   to a `_test.xo` file leaves a build's caches warm. `xo run --watch`
   already restarts only on a changed binary (decision 0267).
6. Inline `test` blocks in non-test files are unchanged: they are parsed
   (they are in the file) and compiled only by `xo test`.

## Consequences

- A build no longer reports errors in test files. Gates that relied on
  `xo build` (or `go run ./scripts/xobuild`, which builds compiler/xo)
  to catch a broken `_test.xo` file must run `xo check` or `xo test`.
  The repository's are: `scripts/xocheck` (`xo check`, which every
  compiler edit uses), `xo test` of the changed module in gates,
  cmd/xo TestNoGCTests (the compiler/check tests) in the hourly run, and
  internal/selfhost TestSelfhostXoTests. The lead's hourly.sh adds
  `xo check compiler/... std/...` so a broken test file elsewhere is
  found within the hour.
- A module that builds can have tests that do not check; `xo test` and
  `xo check` report them, which is where test failures belong.
- Building is faster for modules with large test files (no parse, no
  check, no bindings for test-only imports).

## Alternatives

- **Keep checking test files in a build, but warn instead of fail.**
  Rejected: a build still pays for parsing, checking and binding them,
  and test-only imports still need fetching.
- **A `--tests` flag on build.** Rejected: `xo check` already is that
  command.
