# The AST codec dropped each file's item ranges

Status: Observed (2026-11-06, by reading: the field was missing from the codec)

## Symptom

None that a test caught: a build is correct either way. Found by
reading while chasing three separately compiled internal/arm64 failures
in the decision 0529 step 1 gate (std's parses from the back-end cache
by default), which it did not cause: run alone on linux/arm64 host 1,
TestTracebackSeparate and TestSepcompBackgroundBoth passed with step 1,
with and without this fix, with XO_IFC_CACHE=0 and on main. The cost is
speed: a std record written after a cached parse has no item ranges, so
a build loading it parses whole modules where it parsed single items.

## Cause

compiler/parser/astcodec.xo (decision 0380) calls itself lossless, but it
was written before decision 0428 added File.ranges (each top level
item's source range, which parse_decl parses one item by). encode_parse
never wrote them, so a parse decoded from an ast/ entry had none. std's
record (compiler/check/export.xo, StdFileRec.ranges) copies them from
the parse, so a record written after a cache hit had no ranges, and a
build that loads it parses whole modules (ensure_decl's ensure_tree
fallback) where it parsed single items. The round-trip test compared
the dump, the interface hash and the diagnostics, none of which shows
the ranges.

A scan of every `pub` field of ast.xo's structs against astcodec.xo
(each name read or written in the codec) found File.ranges the only one
missing.

## Fix

AST codec v8 writes and reads File.ranges, and astcodec_test.xo's round
trip compares them. When a field is added to parser.File or
ParseResult, add it to encode_parse, decode_parse and round_trip
(gotchas/new-ast-field-needs-format-and-defhash.md lists the other
places an AST field must reach).
