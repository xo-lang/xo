# A typed nil *Module in a name record panicked `xo typedump`

Status: Observed (2026-10-10)

Symptom: `xo typedump` (and TestSelfhostChecker's Go side) panicked with
a nil pointer dereference in `objText` on a module that uses an import
xo.mod does not require (`use example.com/mono/lib` checked alone, after
`lib.Box{...}`).

Cause: the checker records `c.rec.name(id, imp.Module)` where the import
did not resolve, so `imp.Module` is a nil `*Module`. Stored in an `any`,
it is a typed nil: `obj != nil` is true, and the dump dereferenced it.

Fix: `records.name` drops nil objects of every object pointer type
(`nilObj`, internal/check/info.go); test `TestDumpUnresolvedImport`. Any
new object type recorded through `any` must be added to `nilObj`.
