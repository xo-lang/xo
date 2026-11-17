# std/xmlcodec changes

## 2026-11-16: fix: character data and comments close the open parents, as Go's Marshal

- Kind: fix
- Commit: this push
- Found by: task #90 (module tests against Go's Marshal)
- Go reference: go1.25 encoding/xml marshal.go marshalStruct (s.trim for fCharData and fComment)
- Test: std/xmlcodec_test.xo (fails before)
- Timing: none

`build`'s chardata and comment pieces and `write_chardata` (derive Xml's
encoder) close the parent elements the previous field opened before
writing, as Go's printer does: a struct with `p>b` then `,chardata` then
`p>q>c` is `<p><b>b</b></p>t<p><q><c>c</c></q></p>` (it was
`<p><b>b</b>t<q><c>c</c></q></p>`). The stdcorpus contract had the same
mistake; its rows and the stress evidence rows are regenerated.

## 2026-11-16: feature: depth_check, derive Xml's nesting limit

- Kind: feature
- Commit: this push
- Found by: task #133 (derive Xml decode had no depth limit)
- Go reference: go1.26.1 encoding/xml read.go maxUnmarshalDepth
- Test: golden xml_depth (arm64, LLVM)
- Timing: none

`xmlcodec.depth_check(d)` is "exceeded max depth" from depth 10000, which
derive Xml's generated decoder calls once per nested element.

## 2026-11-10: refactor: one file with its bodies (decision 0588 stage 2)

- Kind: refactor
- Commit: this commit
- Found by: decision 0588 stage 2 (pure modules merged with their implementation)
- Go reference: none (no behavior change)
- Test: internal/stdtest corpus xmlcodec on go, arm64 and llvm

std/xmlcodec.xo now holds the bodies of std/native/xmlcodec.xo (removed), and the Go
backend compiles them from it as its own package. The API, texts and errors
are unchanged: every item the declaration file had is `pub`, with the same
parameter names.

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
