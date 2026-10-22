# 0345: No implicit directory walk; xo run runs exactly one directory

Status: Accepted (owner, 2026-10-22). Supersedes the implicit walk of
decision 0210 (a directory covering the directories below it); 0210's
`DIR/...` pattern and its error forms stay.

## Context

Under decision 0210 a directory argument covered every directory below
it, so `xo check .` and `xo check ./...` were the same. `xo run DIR`
loaded the same set: in a directory holding `a/main.xo` and
`b/main.xo` and no `.xo` file of its own, `xo run .` built and ran
`a/` (measured 2026-10-22 on main `46a1ed503`), a program the command
line never named. The owner sees running code found by walking as an
attack vector: a checkout, a download, or a scratch directory decides
what runs. An unbounded walk was also a resource risk: `xo build /...`
or `$HOME/...` would load every tree below (the agent tools' `--dir`
already had a bound for this reason, 2026-10-21: `xo doc
--dir=/private/tmp` took 220 GB).

## Decision

1. `xo run` never walks. `xo run` means `.`, and `xo run DIR` runs the
   module in exactly DIR. A DIR with no `.xo` file or no `main` is an
   error, exit 1; its note lists the subdirectories directly below
   whose files declare `main` (one level, at most 8), as a hint, and
   never runs one. A `.xo` file argument is an error with the fix
   `xo run DIR` (its directory), exit 2; `xo run DIR/...` stays a
   usage error.
2. `build`, `check`, `test`, `vet`, `doc`, `audit`, `fmt`, `fix`, and
   `parse` cover subdirectories only with `DIR/...`. A plain DIR is that
   directory alone: its module (the checker loads what it imports, and
   `check` the other modules of its `xo.work`), or for `fmt`, `fix`, and
   `parse` its own files. A plain DIR with no `.xo` file is an error
   naming `DIR/...`.
3. Every `DIR/...` walk is bounded by agent.xo's `too_wide` limit,
   shared as `check.MAX_WALK_XO` (6,000 `.xo` files) and
   `check.MAX_WALK_ENTRIES` (60,000 directory entries): past it the
   command fails with an error naming the limit, so `xo build /...` and
   `xo check $HOME/...` refuse. The front end's has-sources test, the
   checker's module walk, and fmt, fix, and parse's file walks all count.

Mechanism: the front end (compiler/xo opts.xo `front`) keeps `DIR/...`
in the arguments; each command's parser turns it into DIR and records
DIR as walked (`core.Ctx.walk`, `Opts.walk`, audit's own flag), and the
checker walks a directory only when it is in `Checker.walk`.
`Checker.walk_all` keeps the editor's and the agent tools' checks of a
workspace or repository root (decision 0289: `xo lsp`, `xo def`, `xo
refs`, `xo query`, `xo edit`) and `xo publish`'s check of the module
being published walking, now within the same bound.

## Consequences

- `xo check .` at a repository root with no `.xo` file of its own is an
  error; `xo check ./...` is the old meaning. Scripts and docs that
  relied on the walk move to `DIR/...`.
- `xo build DIR` builds the program in DIR and what it imports; a module
  below DIR that the program does not import is no longer a root (it was
  checked, not linked, before).
- The workspace modules `xo check` adds from `xo.work` are each that
  directory alone.
- Tests: cmd/xo TestCLIPattern (plain DIR vs DIR/..., `xo run` in a
  parent with two programs, a file argument, the bound).

## Alternatives

- Keep the walk for check and test only (read only commands): rejected
  by the owner; one rule for every command is easier to predict.
- `xo run` picks the only `main` below when there is exactly one: still
  runs code the command line did not name.
- No bound, rely on the user: `/...` and `$HOME/...` are easy typos for
  an agent.
