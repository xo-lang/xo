# Editor support

Last verified: 2026-10-13 (xo 0.9.0; VS Code 1.141.0, Node 22.22.1, vscode-tmgrammar-test 0.1.3, vscode-languageclient 9.0.1)

## VS Code extension (xo-lang/vscode)

Since decision 0516 (owner, 2026-11-05) the extension is its own
repository, xo-lang/vscode (local checkout next to this one,
`../vscode`), with editors/vscode's history: one extension, Marketplace
id `xo-lang.xo`, for syntax (decision 0113), `xo lsp` (0114) and
`xo debug --dap` (0253, 0504; debug type `xo`, launch and attach). Its
README says how to build, test and install it; the points that touch
this repository:

- It finds xo as an installed user has it: the `xo.path` setting, then
  `xo` on PATH, then `bin/xo` of an Xo checkout or `.tools/bin/xo` in a
  workspace folder or above it. Working on Xo with a release xo on PATH,
  set `xo.path` to the checkout's `bin/xo`.
- `npm run check:grammar` there compares the grammar's keywords with the
  newest Xo release's `compiler/lexer/token.xo` (from GitHub at the
  release's tag), or a checkout's with `--lexer PATH`; it replaces
  editors/vscode's grammar_test.go. A change to the lexical structure (a
  keyword, a literal form, a contextual word) updates the grammar and its
  `test/syntax.xo` there, after the release that has it.
- `XO_TEST_XO=bin/xo npm test` there debugs a program through
  `xo debug --dap`; `npm run test:vscode` installs the `.vsix` in a VS
  Code of its own and checks highlighting, hover and a breakpoint.
- Published (owner, 2026-11-05): 0.1.0 on the Visual Studio Marketplace
  and Open VSX as "Xo Language" (the Marketplace refused the display
  name "Xo" as taken), publisher `xo-lang`. Debugging needs Xo 0.40 or
  later (`xo debug --dap`); with an older xo the extension says so when
  a debug session starts.
- Releases: pushing a tag `vX.Y.Z` there runs its release workflow
  (tag checked against package.json and CHANGELOG.md, tests, the grammar
  check, one `.vsix` published to both registries and a GitHub release);
  the steps, the `VSCE_PAT` and `OVSX_PAT` secrets and the yearly Azure
  DevOps token renewal are in its RELEASING.md. The owner tags releases.

Last verified: 2026-11-05.

## `xo lsp` (`compiler/cli/servers/`)

The Go server of `internal/lsp` was deleted with the Go compiler
(decision 0215); its Xo port (decision 0178 stage 5) is what `xo lsp`
runs.

| File | Purpose |
|---|---|
| `lsp.xo` | JSON-RPC framing, session loop, document sync, recheck with overlay (debounced 150 ms), diagnostics, code actions, document symbols, request dispatch. A failed hover, definition, or references answers error -32803 with the load error (a bad `xo.mod`, for example), also sent once as `window/logMessage`. |
| `lsp_features.xo` | Hover, definition, references, rename, formatting, semantic tokens. |
| `lsp_complete.xo` | Completion (trigger `.`: fields, methods, variants, statics, package members; else locals in scope, module names, imports, prelude names, keywords; ranked by the expected type) and signature help (trigger `(` and `,`), over a check of the text patched at the cursor (decision 0219). |
| `resolve.xo` | Every identifier of a file and what it names, and the scope ranges completion reads. |

Tests: `cmd/xo/lsp_test.go` (`TestLSPCompletion`, runs in `-short`)
starts `xo lsp` in a `t.TempDir()` module (`cmd.Dir` and `PWD` both the
module) and checks initialize's capabilities, completion after `p.`,
`http.`, and `r.`, a bare prefix, an empty name, the expected type
ranking, signature help in an open call, and hover and definition into
std. It is the only Go test of `xo lsp` since decision 0215 (the
protocol and corpus tests were in `internal/lsp`). Run it alone with
`go test -run TestLSPCompletion ./cmd/xo` (`XO_CLI` to skip the
rebuild). Last verified: 2026-10-13.

Hand testing without an editor: send `initialize`, `textDocument/didOpen`,
then a request, each framed as `Content-Length: N\r\n\r\n<json>` on the
stdin of `bin/xo lsp`.

Notes:

- TextMate regexes are Oniguruma (lookbehind, backreferences in `end`),
  not Go RE2, so Go cannot compile them; only `npm test` exercises them.
- Raw strings `r#"..."#` close on `"` plus the same number of `#`
  (backreference `\2` in the end pattern).
- Interpolation `${...}` nests braces with a `braces` rule so a map
  literal inside an interpolation does not end it early.
- Contextual words (`derive`, `with`, `from`, `seed`, `after`, `is`,
  `old`) are colored only in their meaningful position, so they stay
  plain as identifiers elsewhere.
- Effect labels are colored only inside a `uses` clause, so a variable
  named `clock` or `net` stays a variable.
- Every `.xo` file in the repo (824 on 2026-10-10, excluding `runs/`)
  tokenizes with no unclosed string or interpolation at end of file.
