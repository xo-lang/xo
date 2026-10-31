# A std default checked when its tree joins was recorded under the caller's file

Status: Observed (2026-10-30, the linux/amd64 host's hourly goldens); fixed.

## Symptom

trace_ring failed to build in 2 of 13 full amd64 golden runs on the
linux/amd64 offload host, with two different errors, and built in every
other run:

- hourly 19:56: `std/http.xo:196:64: error XO1201: not available on the
  native backends (arm64, amd64, LLVM) yet: this literal`, column 64
  being `headers: Map[Str, Str] = {}` of `http.new_request`;
- hourly 09:51: `fault: index -1 out of range for List of length 200 at
  check/comparable.xo:171:27` in `Checker.check_key_types`.

It did not reproduce alone (cold or warm), after each of the 54 goldens
that use std/http, or in three full parallel runs with fresh caches. Full
parallel runs reusing one cache failed log_with every time from the second
run on (`std/log.xo:19:58` and `20:57`, `fields: Map[Str, Debug] = {}` of
`Logger.debug` and `info`), all on the records path (`cache stdx record
loaded`).

## Cause

On the std records path (decision 0388), a std function's declaration
tree can be left pending and joined later by `Checker.need_tree`. No
other pass sees that tree, so its parameter defaults are checked there or
nowhere. Two faults:

- need_tree checked them only for a `lazy_mod` module, so a declaration
  file's function (std/http's `new_request`) was never checked:
  `TestStdRecordDefaultArgs` failed with XO1201 every time;
- where it did check them, it did not set the checker's current file,
  and `check_ty` records each expression's type as a `TypeRec` under
  `cur_file`:

- under another file (the program's main.xo), the `{}` default's Map
  type sat at std/http.xo's offsets in the wrong file, so the SSA
  builder's `type_in(std/http.xo, e)` found nothing and `map_lit` saw a
  non-collection type: XO1201 "this literal";
- under no file (`cur_file` -1), `check_key_types` read
  `c.files[r.file]` with -1 and faulted.

Whether a build took that path, and from which caller, depended on
whether std's records were already in the cache, which a background
`dev stdcache` job writes. In fresh caches a build parsed std whole, so
the bug stayed hidden.

## Fix

`need_tree` checks the defaults of every function whose tree it joins,
under the function's own file (and module and path), restoring the
caller's after (compiler/check/export.xo); an interface's method, with no
file, stays unchecked as on every path. internal/arm64 `TestStdRecordDefaultArgs`
builds a `new_request` call with its default headers from std's record.
The SSA builder also lowers an empty `{}` or `#{}` default with no
recorded type as the empty Map or Set of the parameter's type
(`foreign_value`), as it already did for `[]`, which covers defaults
that are never checked (an interface method's).

A checker step that records types outside the file walk (a lazy join, a
forced body) must set the current file first, as `check_lazy` does.
