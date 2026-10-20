# std/sql changes

## 2026-10-20: feature: bytes, JSON, and array values, and SQLSTATE codes

- Kind: feature
- Commit: (filled in after the push)
- Found by: survey
- Go reference: none
- Test: internal/llvm/testdata/golden/sql_values; internal/llvm/testdata/golden/native_sql

`SqlValue` gains `Blob`, `JsonText`, and `Array` (with `sql.bytes`,
`sql.json`, `sql.array`, and the Row accessors `bytes`, `json`, `ints`,
`strs`, `items`), `derive SqlRow` reads `Bytes` and list fields, and the in
memory driver has `BYTEA`, `JSON`, and `TYPE[]` columns.

## 2026-10-20: behavior: Constraint carries the SQLSTATE and the constraint's name

- Kind: behavior
- Commit: (filled in after the push)
- Found by: survey
- Go reference: none
- Test: internal/llvm/testdata/golden/native_sql; internal/gen/testdata/golden/sql_tx

`SqlErr.Constraint(msg)` is now `Constraint(msg, code, name)` (unique
`23505`, not null `23502`, a value of another type `42804`), and the new
`Server(code, msg)` carries other server errors. A pattern
`Constraint(m)` becomes `Constraint(m, _, _)`.
