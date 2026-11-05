# 0511: `xo check DIR` takes DIR and what it imports, not every workspace module

Status: Accepted (owner, 2026-11-05). Proposed by Xo Lang 1 the same day at the lead's request after aws issue 11. Amends decision 0086 ("xo check ... treat the workspace as one program") and point 2 of decision 0345 ("and `check` the other modules of its `xo.work`") for `xo check` only.

## Context

The AWS SDK (github.com/xo-lang/aws) is one repository of 437 modules:
the core, the codegen and integration modules, and one module per AWS
service, all listed in its `xo.work`. Its issue 11 (docs/reports/
xo-issues.md): `xo check ./service/accessanalyzer`, a small service that
imports only the core, peaks at 8.7 GB resident; the same check with a
workspace of only the core and that service peaks at 0.63 GB.

This is by design. Decision 0086 makes `xo check`, `test`, `edit`,
`query`, `mcp`, and `serve` treat the workspace as one program, and
decision 0345 kept it for a plain `check DIR`: Checker.workspace_dirs
(compiler/check/cli.xo) adds every workspace module the arguments do not
cover. So a one-service check type checks all 435 services. Reproduced
on aws 8fe319e with the 0.39.0 xo (darwin/arm64, 2026-11-05): the check
dies after 18 s with "memory budget exceeded: 18435 MB mapped" at 8.5 GB
resident.

The one-program rule has a reason: checking a core package also checks
every workspace module that uses it, so a breaking change in the core
shows in its dependents. That matters in a monorepo, but its cost grows
with the whole workspace whatever is being checked, and the CLI has
other ways to ask for the whole workspace.

## Decision

1. `xo check DIR` of a plain directory (no walk) checks DIR's module and
   the modules it imports, workspace modules included, transitively, as
   any build does; the workspace's other modules are not loaded.
   Imported workspace modules are still checked, and their errors
   reported.
2. `xo check` takes every workspace module, as 0086 has it, when a named
   directory is the workspace's own directory (the one holding
   `xo.work`; it must hold a module, decision 0345) or a walk of it or
   of a directory above it (`./...` at the workspace root). A walk below
   the workspace's directory (`codegen/...`) checks the modules under
   it, which the walk finds, and their imports, like point 1 (amended
   2026-11-05: `xo check ./codegen/...` in the aws repo took every
   service and stopped at the 30 GB budget; it now takes 0.82 s at
   857 MB on linux/arm64).
3. `xo test`, `query`, `edit`, `mcp`, and `serve` keep 0086's rule: an
   atomic cross-module `xo edit` needs every module.

This is Go's rule for `go vet` and `go test`: they take the named
packages, and `./...` asks for everything.

The code is one function: Checker.check_workspace_dirs, which `xo check`
calls instead of workspace_dirs; it returns the arguments alone unless a
named directory is the workspace's, or a walk of it or of a directory
above it.

## Consequences

Measured on the linux/arm64 offload host, 2026-11-05, branch lang1-ws11
(main f702a22bb plus the change), `XO_CHK_CACHE=off`, peak RSS from
/usr/bin/time:

| check, aws 8fe319e (437-module workspace) | peak RSS | wall |
|---|---|---|
| `./service/accessanalyzer` | 854 MB | 0.80 s |
| the same with a 2-module workspace | 853 MB | 0.78 s |
| `./service/s3` | 1,027 MB | 0.95 s |
| `.` (the workspace: every module) | dies at the 30 GB budget after 54 s | |

Before the change, the first row died at the 18 GB budget on the Mac
(and took 8.7 GB in aws-19's measurement). A plain check's memory no
longer depends on the workspace's size.

New tests (cmd/xo workcheck_test.go): TestCheckWorkspaceImportClosure
(a check of one module passes while an unimported workspace module is
broken; an error in an imported workspace module is still reported; a
walk reports the broken module) and TestCheckWorkspaceSizeMemory (a
one-module check's peak RSS with a 301-module workspace against a
1-module one: 748 MB and 747 MB). TestModules (0086's workspace cases)
passes unchanged.

What changes for users: `xo check ./core` in a workspace no longer
reports breakage in the modules that import the core; `xo check ./...`
(or the workspace's directory) does. CI that relied on the old rule
should check `./...`.

### The whole-workspace check's memory (a follow-up)

`xo check ./...` on the aws repo still cannot finish. Peak RSS of a
whole-workspace check against its size (same host and branch):

| workspace | lines | peak RSS | wall |
|---|---|---|---|
| synthetic, 10 + 1 modules (200 structs and functions each) | | 831 MB | 0.70 s |
| synthetic, 50 + 1 | | 1,072 MB | 0.94 s |
| synthetic, 200 + 1 | | 1,917 MB | 1.87 s |
| aws core + 10 services | 79,068 | 1,199 MB | 1.06 s |
| aws core + 50 services | 680,120 | 4,056 MB | 4.38 s |
| aws core + 100 services | 1,387,263 | 7,366 MB | 8.81 s |
| aws core + 435 services | about 6.5 million | over 30 GB (budget) | |

It is linear in the source checked, about 4.7 KB resident per line on
the aws code: the checker holds every module's checked state (ASTs,
types, objects) until the end. A follow-up is worth doing: check the
members of a whole-workspace check one module at a time (each with its
imports, which are shared and checked once), reporting and freeing each
before the next, so the peak is the largest single module's closure
(S3 and EC2 here) rather than the sum. That needs the checker's state
split per module, or one Checker per member with a shared cache of
imported modules; it is its own decision, after this one.

## Alternatives

- Keep 0086's rule for `check` and only cut the memory (the follow-up
  above): `xo check ./service/x` would still check 435 services, about
  10 s and several GB where 0.8 s and 0.85 GB do, on every edit.
- `xo check --workspace` to ask for the whole workspace: `./...` and the
  workspace's directory already say it, so no new flag.
- Load the other workspace modules lazily, only to parse: the check
  would still not report anything about them, so it is this decision
  with extra cost.
