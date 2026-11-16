# std/jsonrpc changes

## 2026-11-16: fix: decode refuses an error code that is not an integer, and reads a null error message as ""

- Kind: fix
- Commit: this commit
- Found by: azure-1 (TestJsonrpcFuzz, task #115: the differential against the Go MCP SDK)
- Go reference: github.com/modelcontextprotocol/go-sdk v1.8.0 jsonrpc.DecodeMessage (WireError.Code is an int64, Message a string)
- Test: std/jsonrpc_test.xo "an error's code must be an integer, and a null message is no message"; internal/stdtest TestJsonrpcFuzz
- Shipped in: not yet released

decode truncated an error's code of 1.5 (or 2.0, 1e3) to an integer, where the specification
says the code MUST be an integer and the Go SDK refuses the message; it is now INVALID_REQUEST.
And it refused `"message": null` though it reads an absent message as "": a null one is now "" too,
as the SDK reads it. decode's doc now lists where it departs from the SDK on purpose (text after the
message, a null method, an error with no code, a response with neither result nor error, encode of a
response with both), which TestJsonrpcFuzz models.

## 2026-11-11: feature: std/jsonrpc, JSON-RPC 2.0 messages and framings

- Kind: feature
- Commit: this commit
- Found by: decision 0603 stage 1 (Accepted, owner 2026-11-11)
- Go reference: github.com/modelcontextprotocol/go-sdk v1.8.0's jsonrpc package (DecodeMessage, EncodeMessage); the JSON-RPC 2.0 specification's examples for batches and errors
- Test: internal/stdtest/testdata/corpus/jsonrpc (15 specification examples through answer, 29 decode cases, 80 seeded random messages decoded and encoded byte for byte, the framings, Conn over channels, cancellation both ways, Methods) on arm64, LLVM and the Go backend
- Shipped in: not yet released

`jsonrpc.encode`, `decode`, `answer` (batches), the newline and Content-Length framings, and `Conn`, calls and
notifications both ways with requests served in tasks, MCP and LSP cancellation both ways, and `Methods` dispatch (spec/std.md 80). `xo mcp` and `xo lsp` on it are the next step
of stage 1.
