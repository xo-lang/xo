# std/textproto changes

## 2026-11-15: perf: canonical_key checks a key in one pass

- Kind: perf
- Commit: this push
- Found by: the std Proven perf line (bench/perf/textproto canonical 1.257 and header 1.308 of Go on linux/arm64 host 1)
- Go reference: net/textproto reader.go (CanonicalMIMEHeaderKey, validHeaderFieldByte, canonicalMIMEHeaderKey)
- Test: the textproto, mail, mailfuzz, multipart and smtp corpora (go, arm64, llvm); xo bench --ab on linux/arm64 host 1, 3 rounds: canonical 100.6 to 71.9 ms (0.715, about 0.90 of Go from 1.257); header within noise (0.992)
- Affected: none (no behavior change)
- Shipped in: not yet released

canonical_key now scans a key only until the first letter whose case changes (Go's CanonicalMIMEHeaderKey) and builds the rest while checking it, so a key is read once instead of a validity scan, a canonical check and a copy; a header line's key that is already canonical is no longer copied; a field byte is tested against two 64 bit masks instead of a chain of comparisons, and the canonical form skips the UTF-8 check of its output, which is ASCII by construction.

## 2026-11-13: security: read_header is linear in the number of fields

- Kind: security
- Commit: this push
- Found by: the std/http security review (decision 0616), an extra finding (Go's ReadMIMEHeader keeps a map and is linear)
- Test: std/textproto_test.xo "read_header groups interleaved and repeated keys in first-seen order, also at 50000 fields"; measured on arm64: 40000 repeated fields 5771 ms before, 3 ms after
- Affected: every release up to 0.50.1 (all backends); reached by std/mail, std/multipart and std/smtp on untrusted input
- Shipped in: not yet released

read_header added each field to the Header it was building by searching the keys found so far and copying the lists, so a header block of many fields cost time quadratic in their number (a denial of service for a program reading mail or multipart headers it did not write). It now records the fields in order and groups them by key once at the end; keys and values come out in the same order as before.

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

## 2026-11-16: fix: spaces at the end of the input after a line are EOF, and reply line errors quote the line's bytes

- Kind: fix
- Commit: this commit
- Found by: azure-1 (TestTextprotoFuzz, task #115: net/textproto go1.26.11 against std/textproto)
- Go reference: net/textproto reader.go readContinuedLineSlice (a continuation's readLineSlice error is returned), parseCodeLine and readCodeLine (%q of the line's string)
- Test: native/textproto "spaces at the end of the input after a line are io.EOF, as Go's readLineSlice there", "a reply line's errors quote its bytes, as Go's %q"
- Shipped in: not yet released

`read_continued_line` (and so `read_header`) on a line followed by spaces or tabs at the end of the input returned the line;
Go's continuation read there is io.EOF, which ReadContinuedLine returns and ReadMIMEHeader returns with the fields before it
(the field in progress dropped). `read_code_line` and `read_response` quoted a reply line's lossy text in their errors
(U+FFFD for each invalid byte), where Go's `%q` of the line shows `\xHH`; they now quote the line's bytes.
