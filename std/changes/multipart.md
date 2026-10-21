# std/multipart changes

## 2026-10-21: fix: parse_form skips a bad quoted-printable part as Go does

- Kind: fix
- Commit: bc998b26
- Found by: differential test against Go (scripts/stdcorpus multipart, 1,500 generated bodies)
- Go reference: mime/multipart Part.Close and formdata.go readForm; mime/quotedprintable Reader over bufio (go1.26.1)
- Test: internal/stdtest/testdata/corpus/multipart; std/native/multipart_test.xo

A part without a form name that parse_form skips is drained through its
quoted-printable decoder, as Go's Close does: at a bad line the reader
stops after the read that held it, inside the part, and parsing goes on
from there (Go then reports `multipart: unexpected line in Next()`).
Natively the part was skipped to its boundary. 4 of 1,500 bodies differed.

## 2026-10-20: port: std/multipart, Go's mime/multipart in memory

- Kind: port
- Commit: 4de48c2e
- Found by: survey
- Go reference: mime/multipart/multipart.go, formdata.go, writer.go; net/textproto/reader.go readMIMEHeader; mime/quotedprintable/reader.go (go1.26.1)
- Test: std/native/multipart_test.xo (Go's results on 1,400 generated bodies); internal/llvm/testdata/golden/multipart_basic

New module: `parse`, `parse_form`, `write`, `form_data_content_type`, and
`random_boundary`, with Go's results, limits, and error texts on every
backend, and `Request.multipart_form` in std/http.
