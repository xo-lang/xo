# std/multipart changes

## 2026-10-20: port: std/multipart, Go's mime/multipart in memory

- Kind: port
- Commit: 4de48c2e
- Found by: survey
- Go reference: mime/multipart/multipart.go, formdata.go, writer.go; net/textproto/reader.go readMIMEHeader; mime/quotedprintable/reader.go (go1.26.1)
- Test: std/native/multipart_test.xo (Go's results on 1,400 generated bodies); internal/llvm/testdata/golden/multipart_basic

New module: `parse`, `parse_form`, `write`, `form_data_content_type`, and
`random_boundary`, with Go's results, limits, and error texts on every
backend, and `Request.multipart_form` in std/http.
