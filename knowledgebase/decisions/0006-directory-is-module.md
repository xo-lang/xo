# 0006: The directory is the module

Status: Accepted (2026-10-08). Zero violations across single and multi module tasks (08 notes service, 07 cache). The one weakness, directory names that are not identifiers, is handled by 0025.

## Context
Go's `package` clause is redundant with the directory and a frequent source of
small agent mistakes (mismatched names, `main` vs `foo`).

## Decision
No module declaration in source files. A directory is a module; its import
path comes from `xo.mod` plus the directory path.

## Consequences
- One less thing to get wrong. Moving a directory renames the module; `xo edit`
  must handle it.

## Evidence
- Paper test 2026-10-07 ([results](../../bench/results/paper-2026-10-07.md)): No signal: all tasks were single module.
- Paper test 2026-10-07 spec 0.2 ([results](../../bench/results/paper-2026-10-07-v0.2.md)): 0.2 run: first negative signal, a directory name that is not an identifier has no defined module name.
- Paper test 2026-10-07 spec 0.3 ([results](../../bench/results/paper-2026-10-07-v0.3.md)): 0.3 run: module name from a digit leading directory needs `as` (02), handled by 0025.
