# VS Code hover fails: edition "preview" is not a year

Status: Observed (2026-10-13)

Symptom: in VS Code, every hover over a file under `compiler/` pops
"Request textDocument/hover failed." The Xo output channel shows
`compiler/xo.mod: edition "preview" is not a year (want edition YYYY)`,
code -32803.

Cause: the extension started `.tools/bin/xo`, an xo 0.8.0 built on
2026-10-10 by the Go compiler, too old to read `edition preview` (decision 0155).
`go build -o .tools/bin/xo ./cmd/xo`, the documented
way to refresh it, stopped existing with decision 0215, so the binary
silently aged while `bin/xo` (built by `go run ./scripts/xobuild`) moved
on. The server answers a failed hover with an error, and
vscode-languageclient shows each failed request as a popup.

Fix (2026-10-13, `editors/vscode/extension.js`): in an Xo checkout the
extension prefers `bin/xo` over `.tools/bin/xo`, logs the binary and its
`xo version` to the output channel at start, and sends failed requests to
the output channel only (`revealOutputChannelOn: Never`). Run "Xo:
Restart Language Server" (or reload the window) after updating. Elsewhere,
point `xo.path` at a current xo. Check which xo answers with
`<binary> version`: 0.9.0 or later reads `edition preview`.
