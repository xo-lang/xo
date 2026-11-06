# A per member checker copy has no node records for the base's bodies

Status: Observed (Xo Lang 1, 2026-11-05)

## Symptom

`xo check` of a workspace of 8 members or more (decision 0513), with the
support check on (decision 0523), reported thousands of XO1201 "the name
`x` here" in std/native/llrt and in modules two members import, a
different count on each run (aws: 2828, then 863). The same modules
checked alone had none.

## Cause

`Checker.start_member` (compiler/check/dump.xo) makes a copy of the
checked base for each member and drops the base's per-node records
(`types`, `type_at`, `names`, `name_at`), which only the tools read in a
plain check. The SSA builder reads them too: lowering a base body on the
copy found no object for its names. The front end cache hid part of it
(a body the base lowered itself replayed on the member), so the count
followed the cache's state.

## Fix

Anything that lowers or otherwise reads a base body's records on a
member copy calls `start_member_with(true)`, which keeps them (load_each
does when `ctx.member_support` is set). A test of such a path runs with
`XO_FE_CACHE=off` too (cmd/xo TestCheckSupportPerMember), or the cache
can hide it.
