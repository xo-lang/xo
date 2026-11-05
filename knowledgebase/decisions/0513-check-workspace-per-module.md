# 0513: A whole-workspace check holds one member at a time

Status: Proposed (Xo Lang 1, 2026-11-05, at the lead's request; the follow-up decision 0511 names). The prototype is on branch lang1-ws11 behind XO_CHECK_PER_MODULE=1 (not for main).

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

Work, in order:

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
