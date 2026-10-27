# std/xmlcodec changes

## 2026-10-27: perf: derive Xml writes its text with no element tree

- Kind: perf
- Commit: this commit
- Found by: bench/perf/xmlcodec encode's CPU profile (the tree derive Xml built and dropped before encoding it, xml_tree__Order and its leaves about 31%, its drops about 10%)
- Go reference: encoding/xml marshal.go (marshalValue writes to the printer as it walks the struct; parentStack trim and push) (go1.26.1)
- Test: internal/stdtest/testdata/corpus/xmlcodec (a Writer given each comment-free piece list writes, compact and indented, the text build's tree encodes to); internal/stdtest/testdata/corpus/xml (1,500 documents decoded and re-encoded against Go); internal/llvm/testdata/golden/xml_derive (arm64, LLVM); test/errorcheck/derive_xml; twincheck bench/perf/xmlcodec

New: `Writer`, with `writer`, `write_start`, `write_open`, `write_leaf`,
`write_chardata`, `write_end` and `written`: the text `build` and
`Element.encode` give for the same pieces, written as they come, with no
tree. The code `derive Xml` generates (`xml_write__T` in place of
`xml_tree__T`) writes a struct through it. `leaf` and `chardata` keep
their text as given and `build` escapes it. Output is unchanged. Quiet
--ab, 15 rounds, the parent's std and xo as A: bench/perf/xmlcodec
encode wall 0.548 (0.544 to 0.553), instructions 0.598.

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
