# An empty but non nil list prints in the tree dump

Status: Observed (2026-10-10)

Symptom: the self hosted parser's dump of a method recovered from a Rust
`impl` block whose only parameter was `self` lacked the line
`params: []` that the Go parser's dump has.

Cause: internal/ast Text prints a list field that is empty but not nil
as `name: []`, and omits a nil one. The Go parser's `parseImpl` drops the
`self` parameter with `fn.Params = fn.Params[1:]`, which leaves an empty
non nil slice; the Xo tree has no nil lists.

Fix: the Xo `FnDecl` carries `params_list` (compiler/parser/ast.xo), set
by the `impl` recovery, and the dump prints `params: []` for it, as it
already did for a variant's fields, a `use c` ceiling, and pattern
fields. A new recovery in internal/parser that slices or makes an empty
list needs the same treatment; TestSelfhostParserDifferential finds it.
