# 0628: Release a partly used struct subtree by subtree

Status: Accepted (Xo Lang 1, 2026-11-13; lead's memory item)

## Context

The gcp aiplatform/apiv1 cold test build peaked at about 4.5 GB max RSS
(lead, 2026-11-13: find where). On the Mac (`--phase-times`, caches
off) the heap peaked at 3,799 MB at inline.verify; a heap profile there
(XO_PROF=heap, `past phase inline.verify`) held 2.2 GB of the functions'
value tables (ssa.renumber 1.20 GB, arm64.inline_one 1.02 GB; 56 bytes
a value), 1.5 GB of them in tables of 875 to 885 KB.

Those were the package's 919 test functions, 14.2K to 16K values each
(median 15,664; 14.36 million in all) in about 49 blocks. test.702 held
13,534 field reads and 1,876 releases, 13,512 and 1,854 of them in one
run of one block: the drop of `svc = new_service(...)`, the generated
client's service tree (about 280 nested `XService{core, subs...}`
structs, each with its own Core), 1,836 counted leaves.

group_releases (rc.xo) already joins the releases of every leaf of one
root into one release of the root, which the backends lower through a
per type drop helper (bench/results/fast-builds-2026-10-13.md). It
joined only a root whose every leaf the run released; here 24 leaves
(the `svc.custom_jobs.operations` subtree the test's call used) were
released elsewhere, so the run released 1,812 of 1,836 and was kept as
it was: a field read chain and a release per leaf.

## Decision

When a run releases at least GROUP_LEAVES leaves of a root but not all
of them, group_releases releases each complete subtree whole: the
largest paths under the root (or under a call's result) whose every
counted leaf the run releases and that hold GROUP_LEAVES leaves or more
(Rc.sub_cover), each as a chain of field reads to the subtree and one
release of it. The leaves no such subtree covers keep their own
releases. A release in the run covering part of a subtree (some of its
leaves in one, some not) keeps its group as it was. remove_dead_fields
then drops the field reads only the old releases used.

A release of a struct value that is a field of a root was already
emitted (a call result's component, the `comp` group); each backend
lowers it the same way: arm64 and amd64 through the type's drop helper
(rc_helper, isel.xo Release: a whole struct's leaves released from
memory), LLVM through the type's drop function (func.xo Release,
rc_fn("drop"), at the field's address when memory holds it), wasm
through its per type helper (by address past WIDE_RC, 16 leaves); the
Go backend has no counts. The verifier counts a release of a struct as one
of each of its leaves, as for a root.

## Measurements

`xo bench --ab` on linux/arm64 host 1 against the parent (b04dbd328),
cold builds:

| build | n | CPU | wall | max RSS |
|---|---|---|---|---|
| gcp aiplatform/apiv1 test build | 9 | 0.540 (205.2 to 110.8 s) | 0.611 | 0.659 (4.63 to 3.05 GB) |
| the self build (compiler/xo) | 3 | 0.995 (within noise) | 1.000 | 0.981 |

No round was loaded. On the Mac (caches off, XO_DEBUG_VERIFY=1, the
package's 1,344 tests pass) the heap at inline's end went from 3,734
to 1,811 MB; the peak moves to emit and link (3.06 GB, the assembly
text).

__text (Mac, XO_SEPCOMP=0): xo 37,816,444 to 37,687,940 (-0.34%); the
size probe's SDK samples cal 2,028,688 to 2,031,168 (+0.12%), bq
4,281,956 to 4,275,812 (-0.14%), aws 2,434,924 to 2,437,356 (+0.10%):
a subtree's drop helper is emitted once per type, so a program with few
partly used drops can grow by its helpers.

Golden rc_subtree_release (a service tree used partly, moved partly,
and mutated, then dropped): after rc, use_one 23 to 8 releases and 76
to 11 field reads, move_one 11 to 4 and 34 to 7, use_two 51 to 13 and
174 to 21; passes on arm64, LLVM, Go and wasm, with XO_DEBUG_VERIFY=1
and XO_FREE_CHECK=1, XO_LEAKCHECK=1 0 live (arm64, LLVM).

The amd64 sample: 53 of 53, and 53 of 53 again under XO_FREE_CHECK=1
(freed blocks poisoned, so a use after a release faults).

## Consequences

Code that drops a large struct after using part of it (generated
clients' service trees in every SDK, the compiler's own state structs)
releases it in a few calls. The drop helpers of the subtree types are
emitted once per type, as for whole roots.

## Alternatives

- Generated code holding one shared Core behind a reference: fixes one
  generator, not the pattern, and changes the SDKs' API.
- An outlined drop function per root type for any release run: the
  existing per type helpers already are that; the gap was only the
  completeness test.
- Packing ssa.Value from 56 to 40 bytes: about 0.3 GB more, separate
  (planned next as its own change).
