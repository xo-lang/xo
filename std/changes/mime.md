# std/mime changes

## 2026-11-04: perf: format_media_type scans each token once and sorts parameters by one walk

- Kind: perf
- Commit: this push
- Found by: bench/perf/mime format 1.31 of Go's wall (std-twin rows)
- Go reference: mime.FormatMediaType (sorts the parameter names, checks isToken and needsEncoding)
- Test: the mime corpus (arm64, LLVM, Go backend) and `xo test --std-module=mime`
- Timing: host 1 A/B format 0.625 of before (0.605-0.652, n=6), about 0.82 of Go's wall
- Shipped in: not yet released

Each type, attribute and value is classified in one scan (token and lower case, or token and needs encoding), and the parameters are collected in one walk of the map with an insertion sort, in place of `keys().sort()` and a hashed lookup per key.

## 2026-11-03: perf: format_media_type writes a lower case token as it is

- Kind: perf
- Commit: this commit
- Found by: bench/perf/mime format 1.31 of Go's wall on host 1 (bench/results/std-twin-rows-2026-11-03.md)
- Go reference: mime mediatype.go FormatMediaType, strings.ToLower (go1.26.1)
- Test: internal/stdtest/testdata/corpus/mime and mimefuzz (go, arm64, LLVM), TestMimeFuzz

The type, subtype and attribute names are written as they are when they have no upper case letter (they are
tokens, so ASCII), where `to_lower()` copied them. Xo / Go wall of format: 1.313 to 1.284 (quiet `xo bench
--ab`, host 1, 15 rounds).

## 2026-10-28: perf: format_media_type escapes a quoted value in one pass

- Kind: perf
- Commit: this commit
- Found by: bench/perf/mime format at 1.204x Go (measured on main 55af0adc5)
- Go reference: mime.FormatMediaType (one loop over the value, a backslash before each `"` and `\`) (go1.26.1)
- Test: internal/stdtest/testdata/corpus/mime (go, arm64, LLVM); twincheck's checksums agree on none, parse, format, ext, detect

A parameter value that is not a token is written quoted in one pass, the
runs between `"` and `\` copied whole, where two replace calls each
scanned and copied it. Output is unchanged. Quiet --ab, 15 rounds, the
parent's std as A: format wall 0.961 (0.938 to 0.991); Xo / Go about 1.16 (estimate: 1.204 x 0.961), from 1.204, instructions 0.953.

## 2026-10-27: perf: detect_content_type makes a type only on a match

- Kind: perf
- Commit: this commit
- Found by: bench/perf/mime, the std Go twin (detect 1.22x Go; 30% in releasing the empty Str each signature that did not match returned)
- Go reference: net/http sniff.go DetectContentType (go1.26.1)
- Test: internal/stdtest mime corpus; bench/perf/mime twincheck

Each signature's test returns a Bool and the content type is made once,
for the signature that matched. Quiet --ab, 15 rounds: detect wall 0.723
against the parent, 0.885x Go.

## 2026-10-26: perf: detect_content_type matches by offset over constant signatures; type_by_extension is a match

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 2's twin against Go's mime and net/http (bench/perf/mime, std-go-timing-audit)
- Go reference: net/http sniff.go (DetectContentType, sniffSignatures) and mime.TypeByExtension's builtin table
- Test: internal/stdtest/testdata/corpus/mime (go, arm64, LLVM); std/native/mime_test.xo: builtin_type answers every BUILTIN pair; twincheck bench/perf/mime agrees on none, parse, format, ext, detect
- Timing: bench/perf/mime (200,000 passes each), quiet --ab, Xo / Go wall: detect 8.26 to 1.22, ext 5.45 to 0.76, format 1.23 and parse 1.05 unchanged, none 0.67

The signatures are a constant (sigs() built the list, and every HTML
pattern's Bytes, on each call) and are matched by offset into the data
(each HTML pattern sliced a copy of the data from its first non space
byte). type_by_extension is a match generated from BUILTIN (it walked the
list of pairs twice), and an extension with no upper case or non ASCII
byte skips the lowered pass.

## 2026-10-22: perf: parse_media_type copies its tokens of a long value

- Kind: perf
- Commit: 4438a0bf0
- Found by: memory probe (300 values of 1 MiB, one kept token: 318 MB retained)
- Go reference: mime/mediatype.go ParseMediaType (go1.26.1)
- Test: internal/llvm/testdata/golden/views_parsers (arm64, LLVM, go)

Slices are views (decision 0330), so a kept kind or parameter of a long media
type held all of it. Over 4 KiB the kind, names and values are copied: 318 MB
to 6 MB in the probe; shorter values keep slices (copying every token is
+14.7% instructions on a three parameter type, copying only long values
+1.4%; bench/results/views-parsers-clone-2026-10-22.md).

- Kind: perf
- Commit: 2e840b6f
- Found by: XO_PROF=cpu (is_tspecial's fifteen comparisons per byte were 23% of parse_media_type)
- Go reference: mime/grammar.go isTokenChar (go1.26.1)
- Test: internal/stdtest/testdata/corpus/mime (go, arm64, LLVM)

`is_token_char` reads a 256 byte table instead of comparing against the
fifteen tspecials. 200,000 parses of a media type with three parameters,
median of 5, native arm64: 105 ms to 87 ms (Go's package 54 ms). Results are
the same.

## 2026-10-21: fix: detect_content_type finds MP4 on the Go backend

- Kind: fix
- Commit: f72d02b2
- Found by: differential test against Go (scripts/stdcorpus mime: 2,500 media types, 600 formats, 1,500 sniffed prefixes)
- Go reference: net/http/sniff.go mp4Sig (go1.26.1)
- Test: internal/stdtest/testdata/corpus/mime

The Go backend's copy of Go's sniffer had its MP4 brand literal renamed
along with the identifiers (`"mimeMP4"` for `"mp4"`), so MP4 data was
`application/octet-stream` there. The native backends were right.

## 2026-10-20: port: std/mime, Go's media types, extension table, and content sniffing

- Kind: port
- Commit: 4bdf96d7
- Found by: survey
- Go reference: mime/mediatype.go, mime/type.go, net/http/sniff.go (go1.26.1)
- Test: std/native/mime_test.xo (Go's results on mediatype_test.go's inputs, random strings, and sniff_test.go's table); internal/llvm/testdata/golden/mime_basic

New module: `parse_media_type`, `format_media_type`, `type_by_extension`,
`extensions_by_type` (Go's builtin table only), and `detect_content_type`,
with Go's results and error texts on every backend.
