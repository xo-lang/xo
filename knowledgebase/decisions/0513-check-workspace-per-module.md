# 0513: A whole-workspace check holds one member at a time

Status: Accepted (owner, 2026-11-05). Proposed by Xo Lang 1 the same day at the lead's request (the follow-up decision 0511 names). The measurements below were taken on prototypes behind XO_CHECK_PER_MODULE=1 and XO_CHECK_PER_MODULE_BASE=1; main has no variable (see "As built").

## Context

Decision 0511 makes `xo check DIR` of a plain directory take DIR and
what it imports, so the size of the workspace no longer matters there.
A whole-workspace check (`xo check ./...`, or the workspace's own
directory) still loads every member into one checker and keeps every
module's checked state (syntax trees, types, objects, per-node records)
until the end. On the AWS SDK's workspace (aws 8fe319e: the core and 435
service modules, about 6.5 million generated lines) that cannot finish:

| whole-workspace check, linux/arm64, `XO_CHK_CACHE=off` | lines | peak RSS | wall |
|---|---|---|---|
| core + 10 services | 79,068 | 1,198 MB | 1.13 s |
| core + 50 | 680,120 | 4,056 MB | 4.43 s |
| core + 100 | 1,387,263 | 7,366 MB | 8.29 s |
| core + 435 | about 6.5 million | over 30 GB: the memory budget stops it after 54 s | |

Peak memory is linear in the source checked, about 4.7 KB resident per
line. Members of a workspace like this one import the core and std, not
each other, so most of that state is needed only while its own member is
checked.

## Decision

A whole-workspace check checks its members one at a time and holds only
one member's state at once:

1. The **base** is checked once: the prelude, std, and every workspace
   module that two or more members import (the aws core). Its state is
   kept for the whole check.
2. Each member is then checked in a checker that starts as a copy of the
   base (`var c = base`: Xo's value semantics share the base's lists and
   copy one only when the member's check first writes it), with what the
   member alone imports added. The member's diagnostics are kept, and its
   checker is dropped before the next member starts, so its syntax trees
   and records are freed.
3. The diagnostics are merged and sorted as today; a diagnostic of a base
   module is reported once.
4. Checks whose result needs every module at once keep one checker:
   `xo edit` and `xo query` (decision 0086), and any pass that reads
   across members (effects inferred from a skipped module, as
   gotchas/module-check-cache-skips-whole-program-passes.md describes, is
   the known case; a member importing another member puts the imported
   one in the base).

The peak becomes the base plus the largest member's own closure instead
of the sum of all members.

## Consequences

The prototype on branch lang1-ws11 (XO_CHECK_PER_MODULE=1) is the simple
form without a shared base: every member gets a fresh checker that
rechecks the prelude, std and core itself, through the module check
cache when it is on. Same host and workspaces:

| core + N services | whole | per module, cache off | per module, cache on (cold) |
|---|---|---|---|
| 10 | 1,198 MB, 1.13 s | 908 MB, 8.89 s | 664 MB, 3.71 s |
| 50 | 4,056 MB, 4.43 s | 1,169 MB, 43.4 s | 916 MB, 19.1 s |
| 100 | 7,366 MB, 8.29 s | 1,170 MB, 85.8 s | 947 MB, 38.1 s |
| 435 | over 30 GB | 2,290 MB, 399 s | 2,061 MB, 184 s |

And checked from outside, one `xo check ./service/X` process per member
after the core (cache on): the largest closure is EC2's, 2,012 MB, and
the 435 services take 154 s in all.

So the peak is bounded by the largest member's closure, as intended, and
does not grow with the member count: the whole aws workspace, which a
single checker cannot hold in 30 GB, checks in 2.1 to 2.3 GB, with no
error, all 436 modules reported. The prototype's wall is 4.6 to 10
times the whole check's because each member rechecks the prelude, std
and core: that is what the shared base (decision point 1 and 2) removes.
Its expected wall is the whole check's work once (the base once plus
each member once) plus one copy of the base's written lists per member.
That copy is the design's cost to measure first: the Checker has 60
List, Map and Set fields, and a member's check appends to the large ones
(objs, tys, types, files, mods), so each member copies those of the base
once, retaining every element that holds heap memory. If that is too
slow, the base's tables move into a frozen part the member's checker
reads through, with the member's own entries in a second part (an index
past the base's length), so nothing is copied.

### The shared base, measured (2026-11-05, same host and workspaces)

The design's form on the branch: the first member is checked in full and
its checker is the base (Checker.start_member: every module it holds is
marked checked, so run skips their declare, resolve, body and unused
import passes, and its diagnostics, root modules and flow bodies start
empty); each later member is checked from a copy of it
(`var c = base`), and dropped. Cache off:

| core + N services | whole (one checker) | fresh checker per member | shared base |
|---|---|---|---|
| 10 | 1,198 MB, 1.13 s | 908 MB, 8.9 s | 1,048 MB, 1.98 s |
| 100 | 7,366 MB, 8.3 s | 1,170 MB, 86 s | 1,326 MB, 17.6 s |
| 435 | over 30 GB | 2,290 MB, 399 s | 2,400 MB, 80.6 s |

Diagnostics: errors planted in a core package that every service imports
(sigv4) and in two services give the same three diagnostics (code, file,
line) from the whole check and the shared base, each once.

The first form still ran the flow pass over every body, the base's
included, for each member (27% of its profile); a member's copy now starts
with no bodies. What remains of the 2x against a whole check, from a cpu
profile of core + 100: the copy cost the next paragraph names (Map[Int,
Int] puts into maps shared with the base are about 10%, set_type's
records among them), and the passes that still walk the base's tables
(effects, key types, unused names). Both are next: the frozen base with
a member overlay, and those passes limited to the member's own modules.

### Second round (2026-11-05): no base records, shared packages in the base

Three changes on the branch, each measured:

1. A member starts with none of the base's per-node records (each
   expression's type and each name's object, types/type_at and
   names/name_at): they serve the tools, not the member's check, and
   were what the member copied on its first write. core + 100: 17.6 s to
   15.1 s; all of aws: 80.6 s to 69.1 s.
2. The base also checks every package that two or more members import
   and no member holds (scanned from the members' `use` lines and the
   workspace's modules: the aws core packages such as protocol/restjson,
   which the root package does not import and each service re-parsed and
   re-checked): core + 100 to 9.72 s, all of aws to 44.1 s.
3. A member with no `.xo` file of its own is left out, as one checker
   leaves it (found by the new test).

| core + N services | one checker | shared base, second round |
|---|---|---|
| 10 | 1,198 MB, 1.13 s | 968 MB, 1.30 s |
| 100 | 7,366 MB, 8.3 s | 1,235 MB, 9.72 s |
| 435 (all of aws) | over 30 GB, stopped | 2,327 MB, 44.1 s |

At core + 100 the wall is 1.17 times one checker's for a sixth of the
memory, and the whole aws workspace checks. The planted errors give the
same three diagnostics both ways. Tests on the branch (cmd/xo
workcheck_test.go, all pass on linux/arm64):
TestCheckWorkspacePerModuleSameDiags (errors in a package two members
import and in two members: the JSON output of one checker and of the
shared base are equal), TestCheckWorkspaceImportClosure,
TestCheckWorkspaceSizeMemory, and 0086's TestModules.

Left: the overlay (frozen base tables) is not needed at these numbers;
the remaining 17% is the members' own parse and check plus the copy of
the base's other tables, to profile when this is built for main. The
branch runs it behind XO_CHECK_PER_MODULE=1 with
XO_CHECK_PER_MODULE_BASE=1; for main it becomes the default for
whole-workspace checks of more than a few members, with no variable.

### Walks (2026-11-05)

`xo check ./...` at a workspace root now takes the same path on the
branch: the walk's directories outside every member are the base
(checked as named, never walked again), and each member is walked on its
own from a copy of the base. On all of aws 8fe319e (linux/arm64, cache
off): 52.2 s at 3,898 MB, the base 27 directories (the core packages and
the codegen and integration modules' own packages) and 438 members,
474 modules, 0 errors, 4 warnings (XO0305 in codegen/gen/restjson.xo);
one checker stops at the 30 GB budget after 41 to 54 s. With the three
planted errors the walk reports them (sigv4 and two services) and the
same four warnings. TestCheckWorkspacePerModuleSameDiags covers both the
workspace's directory and its walk, and asserts the per module path ran
(XO_CHECK_PER_MODULE_TRACE, a prototype aid).

The walk's peak (3.9 GB) is above the directory form's (2.3 GB), and
not because of its base: keeping in the base only the root packages two
members import (13 of 27; the other 14 checked on their own, unwalked)
gave 3,703 MB and 49.5 s, the same diagnostics. The peak is the
integration member, which imports about ten services, ec2 among them:
`xo check ./integration/realaws/...` alone peaks at 3,415 MB (ec2 alone
2,260 MB). The directory measurements above covered the services only.
So the walk's peak is its largest member's own closure plus the base,
as the design means it to be.

## As built (2026-11-05)

- `xo check` alone: the shared base is used when the check takes the
  whole workspace (the workspace's directory, or a walk of it) and the
  workspace has PER_MODULE_MIN (8) members or more besides the named
  directory; below that one checker is as fast (core + 10: 1.13 s
  against 1.30 s). No environment variable.
- Not for `xo check --support` / `--no-gc`, nor for the other commands
  that load through the same function (vet, doc and the rest): they
  analyze the session's checker, which per member is the last member's
  only (core/cli.xo Ctx.per_module, set by load_check).
- The fresh-checker-per-member prototype is gone; the base is always
  shared.
- cmd/xo TestCheckWorkspacePerModuleSameDiags compares a 4 member
  workspace (one checker) with a 10 member one (per member) holding the
  same three errors, for the directory and its walk: the same JSON.

Work, in order (as planned):

1. Split the base from the members (the import graph of the workspace's
   members, which the build list already reads).
2. Check the base; check each member from a copy of it; merge.
3. Measure on the same subsets: peak RSS (target: base plus largest
   member, about 2 GB on aws) and wall (target: within 1.5 times the
   whole check's at core + 100), then `xo check ./...` on all of aws.
4. Tests: the diagnostics of a whole-workspace check are the same with
   and without it (the cmd/xo workspace tests and a synthetic workspace
   with errors in a base module and in members), and a 300-member
   synthetic workspace's peak RSS stays within noise of a 30-member one.

## Alternatives

- One checker per member with no shared base (the prototype): bounded
  memory, but 4.6 to 10 times the wall.
- Keep one checker and drop each member's bodies after it is checked,
  keeping only its exported declarations: the same memory bound, without
  copies, but the checker's records are keyed by node across modules
  and would need a sweep to remove one module's; the copy needs none.
- Raise the memory budget: the need grows with the workspace (30 GB at
  435 services), so a budget only moves the wall.
