# A struct constant in a std declaration file fails the Go backend's generation

Status: Observed (2026-11-11, Xo Lang 3, std/image's named colors)

Symptom: `go run ./scripts/stdgen` fails with `std/image.xo:128:22: struct
literal of unknown type` for `const BLACK: Color = Color{r: 0, g: 0, b: 0,
a: 65535}` in std/image.xo (a declaration file of a go_compiled module).
Native builds were not reached.

Cause: compiler/gen expr.xo struct_lit resolves the literal's type
through the checked program's objects (g.nobj); the constants of a
declaration file are generated where the file's own struct types are not
registered, so the lookup fails. Constants of Str, Bytes and Int in
declaration files (std/base64.xo) generate fine.

Fix (workaround): a function returning the value (`fn black() -> Color`,
the body in the native file). Next: make gen register a declaration
file's structs before its constants (compiler/gen, near expr.xo:521),
then a struct constant can return.
