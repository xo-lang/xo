# std/textproto changes

## 2026-11-13: behavior: protocol errors quote the line, as Go 1.26.2 and later

- Kind: behavior
- Commit: this push
- Found by: the Go 1.26.1 to 1.26.11 toolchain trial (owner item 9, 2026-11-13: TestGoldenGoParity's http_client_responses differed)
- Go reference: net/textproto reader.go (readCodeLine, ReadCodeLine, readMIMEHeader) and textproto.go (Error.Error: `%03d %q`) (go1.26.11; go1.26.1 printed the text raw)
- Test: the textproto corpus regenerated with go1.26.11, on go, arm64, llvm
- Timing: none (error text only)
- Shipped in: not yet released

`malformed MIME header line`, `malformed MIME header initial line`, `short response`, `invalid response code` and `unexpected multi-line response` show the line as a Go quoted string (`": v"`, a control byte as `\x01`), and a reply's error reads `550 "5.1.1 no such user"`, as Go's net/textproto does since its quoting fix. A program that matched the old text must match the quoted one.

## 2026-11-11: feature: std/textproto, Go's net/textproto over bytes in memory

- Kind: feature
- Commit: this push
- Found by: the std audit of 2026-11-11 (bench/results/std-audit-2026-11-11.md, second gap), decision 0598
- Go reference: net/textproto reader.go, header.go, writer.go, textproto.go (go1.26.1)
- Test: `xo test --std-module=textproto` (native, LLVM; Go's reader, header and writer tests); the textproto corpus on go, arm64, llvm (400 keys, 1500 header blocks, 1200 replies, 1000 dot blocks against Go)
- Timing: none yet (no caller; stage 2 measures multipart with `--ab`)
- Shipped in: not yet released

A new merged, pure module: canonical_key, trim, Header (every value, field order), Reader (read_line, read_continued_line, read_header, read_code_line, read_response, read_dot_bytes, read_dot_lines, rest) and dot_encode, with Go's error texts (TextErr). spec/std.md 77.

## 2026-11-11: fix: trim strips carriage return and line feed (Go's TrimString)

- Kind: fix
- Commit: this commit
- Found by: the resolved std API coverage check: the new HEADER_OPS and TRIMS corpus tables
- Go reference: net/textproto textproto.go TrimString, isASCIISpace (go1.26.1)
- Test: testdata/corpus/textproto TRIMS (against Go's TrimString)
- Shipped in: not yet released

`trim` stripped spaces and tabs only. Go's TrimString strips ASCII space, tab, carriage return and line feed from both ends.
