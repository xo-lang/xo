# A checker list kept every lazily checked body's tree alive

Status: Observed 2026-11-05 (xo-lang-4, the front end memory step Lang 1 parked).

## Symptom

A cold build of s3 and dynamodb peaked at about 1,080 MB, with 236 MB of
parse trees made by `check.Checker.force_body` still in use at the end of
fe.build (a heap profile, `XO_PROF=heap XO_PROF_HEAP_AT=fe.build`). Giving
each built body its lazy span back (`unparse_body`) changed nothing at
first: 3,471 bodies were unparsed and the heap did not move.

## Cause

`check_fn_decl` pushes every checked body onto `Checker.bodies` (a
`BodyRef` with the whole `parser.Block`) for the flow pass, `flow_bodies`.
That pass runs once, in the whole-program check, before the SSA builder
starts. The bodies `check_lazy` checks while the builder builds them
(std/native, decision 0247) were still pushed, never read, and kept their
trees until the build ended.

## Fix

`flow_bodies` empties the list and sets `Checker.flowed`; `check_lazy`
keeps it empty after that. The builder then calls `unparse_body` after a
non-generic body's job, and the tree goes (a later `force_body` parses the
same text again; the checker's records are keyed by position, so nothing
is checked twice). Peak RSS 0.842 (1,108 to 932 MB), wall 0.989.

## Avoiding it

Before freeing a structure, find every list or map that holds a reference
to it: Xo frees at the last reference, so one forgotten holder keeps it
all. A heap profile before and after the change shows whether it went.
