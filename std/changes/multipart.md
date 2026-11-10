# std/multipart changes

## 2026-11-10: refactor: integer bounds as `Int.max` and `Int.min` (decision 0587 point 4)

- Kind: refactor
- Commit: this commit
- Found by: decision 0587 point 4 (std's literal bounds become the type constants)
- Go reference: none (no behavior change; Go's math.MaxInt64 and MinInt64)
- Test: the std module tests and goldens of the gate; the self build

`9223372036854775807` and `-9223372036854775807 - 1` in std/multipart's code are
`Int.max` and `Int.min`; masks, Float literals, comments and texts keep their
digits. No behavior change.

## 2026-11-04: perf: the boundary search of parse is a vectorized find

- Kind: perf
- Commit: this push
- Found by: the twin bench/perf/multipart (parse 1.399 of Go; XO_PROF: read_part 84% of the profile)
- Go reference: mime/multipart's scanUntilBoundary (bytes.Index)
- Test: the multipart corpus and std tests (arm64, LLVM, Go backend); twincheck bench/perf/multipart
- Timing: bench/results/std-twin-multipart-udp-2026-11-04.md: parse 1.399 to 0.222 of Go (n=6), write 0.243
- Shipped in: not yet released

index_of searched with a byte loop that called has_prefix at every offset;
it uses Bytes.find now.

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
