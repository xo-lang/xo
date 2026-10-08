# 0044: Semantic edit rules for xo edit

Status: Accepted (2026-10-08)

## Context
Core 10.4 named three edits (`rename`, `add-field`, `change-sig`) and said
only that they update every reference atomically and are rejected if the
result does not type check. Implementing them needed answers for what
`add-field ... = default` means for the declaration, how call arguments map
to a changed parameter list, and what to do with calls that cannot be
rewritten.

## Decision
- All edits resolve through the checker (`Info.CallOf`, `Info.Ref`,
  `Info.TypeOf`, `Info.VarOf`), refuse a program that does not check clean
  before the edit, and recheck the edited sources in memory
  (`check.CheckOverlay`) before writing any file. Rejection writes nothing
  and reports the diagnostics.
- `add-field S name: T = d` declares `name: T` without a default and writes
  `name: d` into every struct literal. The default is a migration value: an
  agent that wanted a permanent default can add the field by hand, and
  literals then stay valid without the edit.
- `change-sig` matches arguments to new parameters by name, pairs a removed
  and an added parameter at the same position as a rename (also renaming
  body uses), keeps argument order (evaluation order) and makes moved
  arguments named. It never drops an argument or invents a value for a
  parameter without a default: those calls reject the edit and are listed
  in `sites`.
- `change-sig` replaces parameters, result, and error type only; clauses
  are edited by hand.
- The summary is always JSON; exit 1 means rejected, 2 a usage error.

## Consequences
- An edit never leaves the module broken and never writes half of a change.
- Dropping arguments for removed parameters is always a manual step, even
  when the argument has no effects.
- Types, fields, and interface methods cannot be renamed yet (type
  references in `ast.Type` nodes are not recorded by the checker).

## Alternatives
- Keep `= d` as the declared field default: literals would need no change,
  but the default would become permanent API, and the spec example
  (`Time.UNIX_EPOCH`) reads as a backfill value.
- Reorder arguments to the new parameter order: shorter calls, but changes
  evaluation order of arguments with effects.
- Drop arguments of removed parameters: mechanical, but silently loses
  effects.
