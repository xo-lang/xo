# Editor support

Last verified: 2026-10-10 (VS Code 1.141.0, Node 22.22.1, vscode-tmgrammar-test 0.1.3, vscode-languageclient 9.0.1)

## VS Code extension (`editors/vscode/`)

Decisions 0113 (TextMate grammar) and 0114 (`xo lsp`). The extension
colors files with the grammar and starts `xo lsp` for diagnostics,
quick fixes, hover, definition, references, rename, document symbols,
semantic highlighting, and formatting.

| File | Purpose |
|---|---|
| `package.json` | Registers language `xo` for `.xo`, points at the grammar. Publisher `xo-lang`. |
| `language-configuration.json` | `//` comments, brackets, auto closing pairs, indentation. |
| `syntaxes/xo.tmLanguage.json` | Grammar, scope `source.xo`. Built from spec/core.md section 1. |
| `syntaxes/xo.markdown.tmLanguage.json` | Injection: ` ```xo ` fences in Markdown use `source.xo`. |
| `extension.js` | Starts `xo lsp` with vscode-languageclient 9.0.1. Binary: setting `xo.path`, else `.tools/bin/xo` in or above a workspace folder or the active file, else `.tools/bin/xo` of the checkout the extension is symlinked from, else `xo` on PATH (2026-10-10: the first version only checked workspace folder roots and failed for a folder outside the checkout). Command "Xo: Restart Language Server". Setting `xo.lsp.enabled`. |
| `test/syntax.xo` | Tokenization assertions (`// ^^^ scope` lines), run by `npm test`. |
| `grammar_test.go` | Go test: grammar keywords equal `token.Keywords()`, manifest matches grammar. Runs in `go test -short`. |

Install for local use (picks up edits on window reload, no packaging):

```
go build -o .tools/bin/xo ./cmd/xo                  # the server the extension starts
(cd editors/vscode && npm ci)                       # vscode-languageclient
ln -s "$PWD/editors/vscode" ~/.vscode/extensions/xo-lang.xo-0.0.1
# then in VS Code: Developer: Reload Window
code --list-extensions --show-versions | grep xo   # xo-lang.xo@0.0.1
```

Uninstall: `rm ~/.vscode/extensions/xo-lang.xo-0.0.1` (the symlink only).

A shareable `.vsix`: `cd editors/vscode && npx @vscode/vsce package
--allow-missing-repository --skip-license`, then
`code --install-extension xo-0.0.1.vsix`.

Rebuild `.tools/bin/xo` after compiler changes, then run "Xo: Restart
Language Server" (or reload the window).

Tests:

```
go test -short ./editors/...            # keyword drift against the lexer
go test ./internal/lsp/                 # protocol, every feature, a corpus panic check
XO_LSP_CORPUS=../../bench:../../internal:../../std go test -run TestCorpus ./internal/lsp/  # the whole repo, about 1 min
cd editors/vscode && npm ci && npm test # tokenization assertions (needs Node)
```

The Markdown injection has no automated test; it was checked on
2026-10-10 by tokenizing a fence with vscode-textmate against VS Code's
own `markdown-basics` grammar.

When the lexical structure changes (a keyword, a literal form, a
contextual word, a predeclared name), update the grammar and
`test/syntax.xo` in the same change as `spec/core.md`. The builtin type
and interface lists in the grammar mirror `protectedNames` in
`internal/check/decl.go`; only keywords are checked automatically.

## `xo lsp` (`internal/lsp/`)

| File | Purpose |
|---|---|
| `jsonrpc.go` | Content-Length framing, message types. |
| `server.go` | Session loop, document sync (full and incremental), recheck with overlay, diagnostics, hover, definition, references, rename, code actions. |
| `resolve.go` | Every identifier of a file (reflection walk, unlike `check.Inspect` it covers declarations, types, patterns) and what it names, from `check.Info`. |
| `features.go` | Semantic tokens, document symbols, formatting. |
| `text.go` | Byte column to UTF-16 conversion, file URIs. |

Hand testing without an editor: send `initialize`, `textDocument/didOpen`,
then a request, each framed as `Content-Length: N\r\n\r\n<json>` on the
stdin of `.tools/bin/xo lsp`.

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
