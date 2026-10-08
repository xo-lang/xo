# In `xo mcp`, anything written to os.Stdout corrupts the protocol

Status: Anticipated (2026-10-08).

Symptom: an MCP client reports a parse error or drops the session after a
tool call.

Cause: `xo mcp` speaks JSON-RPC on stdout and runs the CLI commands in
process. A command that writes to `os.Stdout` directly (rather than the
`stdout` writer it was given) or a child process that inherits the real
stdout lands in the middle of the message stream.

Fix: CLI commands must write only to their `stdout` and `stderr` parameters,
and pass them to child processes (`execBinary` does). New tools in
internal/mcp should go through `Server.exec`, which captures both.
