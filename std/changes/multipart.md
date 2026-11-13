# std/multipart changes

## 2026-11-13: security: a part's header is read in time linear in its fields

- Kind: security
- Commit: this push
- Found by: Lang 2's review of the std/http security fixes (decision 0616); no Go advisory (Go's textproto keeps a map)
- Test: std/multipart_test.xo "a part's header of many fields keeps every value in order (read linearly)"; measured on arm64: a 989 KB form of 10 parts of 9999 fields, 1364 ms before, 13 ms after
- Affected: every release up to 0.50.1 (all backends: parse, parse_form, Reader)
- Shipped in: not yet released
- Remote trigger: any client posting a multipart body to a route that parses it (a denial of service: CPU)

Each field of a part's header searched the keys found so far, so a part of up to the 10000 field limit cost time quadratic in its fields, about 1.4 s of CPU per MB of form. The fields are now recorded in order and grouped by key once; keys and values come out in the same order as before.

## 2026-11-13: security: a streamed reader's header line stops at the header limit while it is read

- Kind: security
- Commit: this push
- Found by: the std/http security review (decision 0616), against GO-2024-2599 (CVE-2023-45290)
- Go reference: net/textproto reader.go readLineSlice(lim) (the limit applied while the line is read), fixed in go1.22.1
- Test: std/multipart_test.xo "a streamed header line stops at the header limit while it is read" (fails before the fix: all 64 MiB read)
- Affected: releases with multipart.reader (decision 0310 phase 3) up to 0.50.1, all backends
- Shipped in: not yet released

`multipart.reader` read a part's header line until its newline whatever its length, copying what it had so far with each piece the source gave, and only then checked the 10 MiB header limit. A client streaming one long header line made the server hold and copy all of it (a denial of service class). The line now stops just past the limit, and a long line is gathered once instead of being copied again with each piece.

- Remote trigger: any client of a `handle_stream` route that reads a multipart body with `multipart.reader`.

## 2026-11-13: behavior: a malformed header's error quotes the line, as Go 1.26.2 and later

- Kind: behavior
- Commit: this push
- Found by: the Go 1.26.1 to 1.26.11 toolchain trial (owner item 9, 2026-11-13: TestGoldenGoParity's http_client_responses differed)
- Go reference: net/textproto readMIMEHeader (go1.26.11; go1.26.1 printed the text raw)
- Test: the multipart corpus regenerated with go1.26.11, on go, arm64, llvm
- Timing: none (error text only)
- Shipped in: not yet released

A part header's `malformed MIME header line` and `malformed MIME header initial line` errors show the line as a Go quoted string (`"Content-Type: a\x01b"`), the key still canonical.
## 2026-11-13: security: parse_form's header limit never goes below zero (GO-2026-6608)

- Kind: security
- Commit: this push
- Found by: the std/http security review (decision 0616), against GO-2026-6608
- Go reference: net/textproto reader.go readMIMEHeader and mime/multipart formdata.go readForm (the fix for GO-2026-6608)
- Test: none fails before the fix (the result is the same "message too large"); std/multipart_test.xo's parse_form tests cover the limits
- Affected: every release up to 0.50.1, all backends
- Shipped in: not yet released

When fewer than 400 bytes of `parse_form`'s memory budget were left at the start of a part, the part's header lines were read with no length limit, as in Go before its fix. In Xo the form is already in memory (at most the server's 64 MiB body), so the cost was one extra copy of that line before the same "message too large" error. The limit is now clamped at zero.

- Remote trigger: a client posting a multipart form, with a bounded cost.

## 2026-11-11: fix: a bad header value's error shows the key canonical, as Go's

- Kind: fix
- Commit: this push
- Found by: decision 0598 stage 2
- Go reference: net/textproto (go1.26.1)
- Test: `xo test --std-module=multipart` (a regression test with Go 1.26.1's text); the multipart corpus on go, arm64, llvm
- Timing: none (header and DATA handling, not a hot path)
- Shipped in: not yet released

Go's readMIMEHeader canonicalizes a key in place before it checks the value, so `content-type: a\x01b` fails with `malformed MIME header line: Content-Type: a\x01b`; multipart showed the key as written. PartHead.header_values canonicalizes its name with std/textproto.canonical_key.

## 2026-11-10: feature: Reader, Go's multipart.Reader over a streamed body; merged, one implementation

- Kind: feature
- Commit: this push
- Found by: decision 0597 part 3 (std/native/http used the streaming parser through a std-internal `pub`)
- Go reference: mime/multipart NewReader, Reader.NextPart, Part.Read, Part.Header (go1.26.1)
- Test: std/multipart_test.xo (the reader against parse on every PARSE body at 1, 7 and 4096 bytes a read; a source error; header_values), golden multipart_reader on arm64, LLVM and Go; the multipart corpus on go, arm64 and llvm; native/http's tests; goldens http_form, http_multipart_stream, multipart_basic
- Shipped in: not yet released

`multipart.reader(src, boundary)` reads parts as the body arrives: `src` gives the next bytes, None at
the end, or its error's text; `next_part` is NextPart (a `PartHead`: name, filename, the first value of
each header, and `header_values` for all of one), `read(max)` is Part.Read. It is effect generic
(`Reader[fx]`), so `src` may read a connection. `http.BodyReader.multipart` uses it. A source error
in a part's headers is now that error (it ended the parts as Go's io.EOF before). std/multipart.xo holds
the bodies; the Go backend runs them too, generated into std/rt (std/rt_hosted.txt), in place of
std/rt/multipart.go over Go's package.

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
