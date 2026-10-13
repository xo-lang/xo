# 0210: The ./... package pattern

Status: Accepted (owner, 2026-10-13). Implemented in selfhost/ (xo1);
the Go `cmd/xo` is frozen (decision 0178 stage 7 step 1) and does not
take the pattern.

## Context

`xo check ./...` failed with `stat ./...: no such file or directory`,
which surprises anyone coming from the go command, and an agent writes
`./...` by habit. The owner decided (2026-10-13) that xo accepts the
pattern, with Go's semantics as the model.

What a directory argument already means (measured on xo1, 2026-10-13,
on a module with `sub/`, `sub/deep/`, `testdata/x/`, `.hid/`, `_skip/`,
and `vendor/v/`): every command that takes directories already covers
the directories below the one given. `xo check .` checked every directory
with `.xo` files at or below `.` as its own module (5 modules), skipping
`testdata` and names starting with `.`, but not `_skip` and not a
`vendor/` without `modules.txt`; `xo test .` ran the tests of `sub/`
too; `xo build .` and `xo vet .` and `xo doc .` load the same set
(selfhost/check `find_module_dirs`). `fmt` and `fix` walk the files
below the directory (fmt includes `testdata`; parse walks everything).
Unlike Go, where `.` is one package and `./...` all packages below it,
in xo `.` and `./...` overlap entirely.

## Decision

- `DIR/...` (`./...`, `sub/...`, `/abs/...`) is accepted wherever a
  command takes directories: check, test, build, vet, doc, fmt, fix,
  parse, audit. It means DIR: every directory at or below DIR, which is
  what DIR already means to each of those commands, so `xo check ./...`
  and `xo check .` print the same, byte for byte. Workspaces (xo.work)
  apply to it as to DIR.
- Directories whose name starts with `_` are now skipped by every walk
  that skips `.` names (the checker's module discovery, the module
  commands' walks, fix, and fmt), as the go command skips them, so `.`
  and `./...` agree on them too. `testdata` and `.` names were already
  skipped; `vendor/` stays skipped as before (the checker: a vendor
  directory with `modules.txt`; the module commands: any `vendor`).
- A `DIR/...` whose DIR exists but has no `.xo` file at or below it
  (with those skips) is an error: `xo CMD: DIR/... matched no directory
  with .xo files`, exit status 1 (go only warns; an agent's typo should
  not pass silently). A DIR that does not exist is reported by the
  command as for DIR. The message has no XO code: the command line's
  usage and file errors are plain text on stderr, not diagnostics.
- Other forms (`...` alone, `a/.../b`, `name...`) are usage errors
  (exit 2) that name the supported form. `xo run` takes one directory,
  so `xo run ./...` is a usage error.
- The expansion is in xo1's front end (selfhost/xo opts.xo `front`),
  before any command runs, so every command gets plain directories.

## Consequences

- `go`-style command lines work: `xo check ./...`, `xo test ./...`,
  `xo fmt --check ./...`, `xo vet ./...`, `xo build ./cmd/...`.
- `xo build ./...` builds one program from everything below `.`, as
  `xo build .` does, not each main package separately as `go build`
  would.
- A module with a `_`-prefixed directory of `.xo` files no longer
  checks it; the Go `cmd/xo` still does (no repository directory starts
  with `_`, so the differential tests are unaffected).
- Tests: cmd/xo TestCLIPattern (and TestCLIHelp for the usage errors)
  against the shipped xo.

## Alternatives

- Go's exact model (`.` one package, `./...` every package): would
  change what `xo check .` means for every existing user and script.
- Reject `./...` with a hint (the first plan of 2026-10-13): superseded
  by the owner's decision.
- Expand `DIR/...` into the list of directories: the commands walk below
  each directory given anyway, so the list adds nothing and would make
  each directory a separate root (and a separate program for build).
- A warning and exit 0 when nothing matches, as go does: a mistyped
  directory would pass silently in an agent's loop.
