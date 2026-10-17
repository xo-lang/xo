# The Go generator imported a package it only compared types of

Status: Observed (2026-10-12, full suite on main dbb8b05), fixed.

## Symptom

`TestGeneratedGoVetsLarge/.._selfhost_xo`: go build failed with
`"xoprog/m/x_github_com_xo_lang_xo_selfhost_lexer" imported and not
used`. compiler/xo has no `use` of compiler/lexer; it reaches lexer's
`Span` and `Pos` through compiler/parser's aliases
(`td.span.start.line`).

## Cause

`pkgRef` records an import whenever `goType` prints a type of another
module, and `goType` is also called where the text is only compared or
analyzed, never written: `coerce` (`ft == tt` for a selected field
captured inside a string interpolation in a struct literal), the range
analysis (ranges.go `isInt`, `typeRange`, `isIntegerType`), and the
adapter registry. A module that never names the type in its own text
still got the import. Regression golden:
internal/gen/testdata/golden/transitive_import.

## Fix

The import block of a module's package lists another module's package
only when the package text names it (`name.` not preceded by an
identifier byte: gen.go `namesPkg`, compiler/gen `names_pkg`). Calls of
`goType` for comparison may keep recording imports; the file step
drops the ones the text does not use.
