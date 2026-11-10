# A workspace member's check installed std's record a second time

Status: Observed (2026-11-09)

## Symptom

`xo check ./...` over the azure SDK workspace (its azotel branch, 362
modules) reported 1,350 to 1,767 errors such as "expected `Str`, found
`Str`" and "expected `Bool`, found `Bool`", in every member that imported
a std module the shared base had not. The count changed with the cache's
state. The first check with a newly built xo was clean, as was every
check with `XO_CHK_CACHE=off` or `XO_STD_EXPORT=0`, and a check of one
member alone. The memory use was normal (0.7 GB).

## Cause

A walk of 8 or more workspace modules checks each member on a copy of a
base checker (decision 0513, cli/core/check.xo load_each). When a member
imported std modules the base lacked, check/dump.xo run() took the
std record path again: it decoded std's whole record (every std object,
the prelude's included) and installed it after the base's objects. The
member then held two `Str` objects, two `Bool` objects and so on. Named
types are equal only when their objects are (check/infer.xo), so every
value that crossed between the two copies failed to unify. The record is
only in the cache after a first check writes it, which is why a cold run
was clean.

Builds never check per member (only `xo check` sets per_module), so build
output was not affected. The duplicate objects only make types unequal,
so the bug rejected good code; it could not make the checker accept bad
code.

## Fix

A member's checker (base_mods not empty) no longer uses the record: the
std modules its base lacks are declared from their sources, the path a
module missing from the record already took. Regression test:
cmd/xo TestCheckWorkspaceMemberStdRecord (9 members importing std/html,
checked twice).

## How to avoid

A checker copied from a base must add only what the base lacks. Any
table that holds "everything" (std's record, an all-modules cache entry)
must not be installed twice into one object table.
