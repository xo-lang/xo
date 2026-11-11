# std/textproto changes

## 2026-11-11: feature: std/textproto, Go's net/textproto over bytes in memory

- Kind: feature
- Commit: this push
- Found by: the std audit of 2026-11-11 (bench/results/std-audit-2026-11-11.md, second gap), decision 0598
- Go reference: net/textproto reader.go, header.go, writer.go, textproto.go (go1.26.1)
- Test: `xo test --std-module=textproto` (native, LLVM; Go's reader, header and writer tests); the textproto corpus on go, arm64, llvm (400 keys, 1500 header blocks, 1200 replies, 1000 dot blocks against Go)
- Timing: none yet (no caller; stage 2 measures multipart with `--ab`)
- Shipped in: not yet released

A new merged, pure module: canonical_key, trim, Header (every value, field order), Reader (read_line, read_continued_line, read_header, read_code_line, read_response, read_dot_bytes, read_dot_lines, rest) and dot_encode, with Go's error texts (TextErr). spec/std.md 77.
