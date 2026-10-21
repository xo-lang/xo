# std/mime changes

## 2026-10-21: fix: detect_content_type finds MP4 on the Go backend

- Kind: fix
- Commit: (filled in after the push)
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
