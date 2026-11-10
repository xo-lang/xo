# A merged std module loses a native override of a derived method

Status: Observed (2026-11-10, Lang 3, std/protoparse; found by Lang 2 and the lead)

## Symptom

After a decision 0588 merge, a type's text changes: `xo proto gen` printed
`bad.proto:ParseErr{line: 2, col: 13, msg: "..."}` where it printed
`bad.proto:2:13: ...`. cmd/xo TestProtoGen failed on main.

## Cause

In the split layout the declaration file says `struct ParseErr { ... }
derive Display` and std/native/M.xo defines `fn parseerr_display(e)`, which
the native backends use in place of the derived method (the `type_method`
naming rule). Merged, the derive is the only `display`, and
`parseerr_display` is dead code nobody calls; no checker warns.

## Fix

In the merged file drop the derive and write the method:
`pub fn (e: ParseErr) display() -> Str { ... }`. Lang 3's merge script
(~/xo-lead/notes/lang3/merge_mod.py) now refuses a module where a derived
type has a native `T_display` (or `_eq`, `_cmp`, `_hash`, `_debug`). After
any merge, check for a non-pub `fn type_method(` the merged file never calls.
