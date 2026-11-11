# std/jsonrpc changes

## 2026-11-11: feature: std/jsonrpc, JSON-RPC 2.0 messages and framings

- Kind: feature
- Commit: this commit
- Found by: decision 0603 stage 1 (Accepted, owner 2026-11-11)
- Go reference: github.com/modelcontextprotocol/go-sdk v1.8.0's jsonrpc package (DecodeMessage, EncodeMessage); the JSON-RPC 2.0 specification's examples for batches and errors
- Test: internal/stdtest/testdata/corpus/jsonrpc (15 specification examples through answer, 29 decode cases, 80 seeded random messages decoded and encoded byte for byte, the framings) on arm64, LLVM and the Go backend
- Shipped in: not yet released

`jsonrpc.encode`, `decode`, `answer` (batches), the newline and Content-Length framings (spec/std.md 80). The
connection (`Conn`), and `xo mcp` and `xo lsp` on it, are the next step of stage 1.
