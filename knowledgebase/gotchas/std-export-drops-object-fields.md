# A checker object field missing from the std export is lost when std is read back

Observed 2026-11-16.

## Symptom

TestDevErrorcheck failed on test/errorcheck/std_internal_fields: the XO0605
errors on std/uuid's `Uuid` fields came without their note ("the fields of
`uuid.Uuid` are private to std (decision 0440); use `u.bytes()` and
`uuid.from_bytes(b)`..."). It started with 4ccafc37c (std/uuid v3, v5 and
hash), which changed nothing in the checker: with its new `use std/crypto`
and `NAMESPACE_*` constants, uuid came from the std export instead of being
declared from source, and either change alone did not show it.

## Cause

compiler/check/export.xo writes each Obj field by field (`write_obj`,
`read_obj`). `ObjFlags.std_internal` and `Obj.internal_hint` (decision 0440)
were never added, so an `@internal` struct read from the export had neither:
the fields stayed hidden (field_hidden needs no flag), but `internal_note`
returned early.

## Fix

The export writes and reads both (EXPORT_VERSION v14), and
compiler/check/export_test.xo checks the round trip of an `@internal`
struct. A new Obj field or flag is added to write_obj and read_obj, with the
version bumped, in the same change; a test that declares from source does
not see the omission.
