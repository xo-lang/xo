# std/xmlcodec changes

## 2026-10-25: fix: float_of names an overflowing float as out of range

- Kind: fix
- Commit: (filled in after the push)
- Found by: the xmlcodec corpus's float inputs against Go's ParseFloat
- Go reference: strconv atof.go (special, readFloat, underscoreOK) (go1.26.1)
- Test: internal/stdtest/testdata/corpus/xmlcodec (1e309, -1e309, 1e100000, a 400 digit mantissa, and the syntax rows; go, arm64, LLVM)

`float_of` answered every failing parse with `invalid syntax`. Go's
`ParseFloat` says `value out of range` for a text that is well formed but
overflows, so a decoded float field reported the wrong reason. It now
tells the two apart the way `int_of` already does: when `Float.parse`
answers None, a text that `ParseFloat` would accept (its `inf`, `infinity`
and `nan` words, decimal and hexadecimal forms, and underscore rule,
`float_syntax` in std/native/xmlcodec.xo) is out of range and anything
else is a syntax error. `Float.parse` itself is unchanged: it still
answers None for both, as Go returns an error for both.
