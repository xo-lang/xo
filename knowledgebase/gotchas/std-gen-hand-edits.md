# std_gen.go edited by hand drifts from the generator

Status: Observed (2026-10-20)

## Symptom

`std/rt/std_gen.go` (the Go backend runtime's declarations of the std
types) was maintained by hand from the deletion of the Go compiler
(decision 0215, 2026-10-13) to 2026-10-20, because its generator
(`internal/gen.StdTypes`) went with it. Regenerating it showed:

- five hash seeds (Http_Cookie, Http_SameSite, Http_CookieErr,
  Tls_Verify, Tls_TlsErr) that were not the FNV-1a of the type name
  shifted right, the rule every other type follows;
- `Sql_DriverRows` without the `XoMark` method the generator gives a
  struct holding lists, which the Go backend's in place list writes
  (unshared lists) rely on to mark the lists a copy shares;
- `Uuid_Uuid` without the JSON methods every newtype has (core 2.8);
- the declarations of ten modules appended at the end of the file in
  arrival order, and two struct fields only the runtime reads
  (Http_Request's XParams and others) that no declaration lists.

## Cause

Copying a nearby type's methods and editing names and constants by hand.
No test compared the file with the declarations.

## Fix

`go run ./scripts/stdgen` writes the file (compiler/gen stdtypes.xo
through `xo dev dump go --std-types`). internal/repotest's
`TestStdGenCurrent` (-short) fails when std/*.xo's type declarations
changed since, and internal/gen's `TestStdGenOutput` when the file
differs from the generator's output. The runtime-only fields are listed
in stdtypes.xo `rt_extra_fields`. Never edit the file by hand; a merge
conflict in it is resolved by regenerating it after the rebase.

## Since 2026-10-21

The generated declarations are one file per std module
(`std/rt/std_gen_M.go`, `std/rt/std_sigs_M.go`), because the shared
file conflicted on almost every rebase of a std push that touched a
type (csvpar, tmpl, and mailmsg on 2026-10-21). A conflict in one of
them is still resolved by `go run ./scripts/stdgen` (or
`go run ./scripts/stdsync`, which does it beside spec/std.md's prelude
index and the habit goldens), and `stdgen -check` also fails on a file left
over from a removed module; its message names stdsync.
