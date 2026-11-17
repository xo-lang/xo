# std/jsonrpc changes

## 2026-11-17: fix: a cancelled call no longer cuts its request short, which deadlocked the connection

- Kind: fix
- Commit: this commit
- Found by: xw-1 (stress evidence on linux/arm64 host 2, task #152; fixed under task #196)
- Go reference: github.com/modelcontextprotocol/go-sdk v1.8.0 jsonrpc2 (a write checks its context only before it starts, so a message is never cut short)
- Test: internal/stdtest/testdata/corpus/jsonrpc "Conn: a call cancelled while its request is half written still writes all of it" (fails before: the rest of the request never reached the peer); std/evidence/jsonrpc/stress fails 6 of 6 runs before (`frame: jsonrpc: no Content-Length header`, then XO-F001) and passes 6 of 6 after on arm64 and LLVM
- Shipped in: not yet released

A call whose task was cancelled while its write function was partway through a message (a transport writing a frame
in pieces, each a cancellation point) unwound there, leaving half a frame on the transport: the peer's reader could
not read another message, its read loop ended, and every call on both sides waited forever. While `serve` runs, a
writer task in its group now writes every message, in order; a sender, cancelled or not, waits for its message's
result, and a cancelled one stops waiting while the message still goes out whole. Before `serve`, the sender writes
as before. Each message sent while serving now hands off to that task (bench/perf/jsonrpc times encode and decode
only, so it does not measure Conn).

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
