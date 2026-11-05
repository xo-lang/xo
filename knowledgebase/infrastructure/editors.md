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
- Releases: see "Releasing the VS Code extension" below.

Last verified: 2026-11-05.

## Releasing the VS Code extension

Owner, 2026-11-05. The extension is released from its own repository,
[xo-lang/vscode](https://github.com/xo-lang/vscode), whose RELEASING.md
has the same steps.

- Names: the extension's display name is "Xo Language" (the Marketplace
  refused "Xo" as taken), its id `xo-lang.xo`. The Visual Studio
  Marketplace publisher is `xo-lang` (display name "Xo language"); the
  Open VSX namespace is `xo-lang`. 0.1.0 was published by hand on
  2026-11-05.
- Tags are the owner's: an agent prepares the version and changelog
  but never pushes a release tag.
- Pushing a tag `vX.Y.Z` runs `.github/workflows/release.yml` there:
  `scripts/release-check.js` (the tag must be `v` plus package.json's
  version, and CHANGELOG.md must have a `## X.Y.Z` section that is not
  marked unreleased; its text becomes the release notes), `npm ci`,
  `npm test`, the grammar check, `vsce package` once, then that same
  `.vsix` to the Marketplace (`vsce publish --packagePath`), Open VSX
  (`ovsx publish`) and a GitHub release. The workflow has `contents:
  write` only; its actions are pinned to commits, vsce and ovsx to exact
  versions in package-lock.json.
- The release commands (0.1.1 as the example), in the vscode checkout:

  ```sh
  npm version 0.1.1 --no-git-tag-version
  # CHANGELOG.md: add "## 0.1.1 (YYYY-MM-DD)" at the top with the changes
  node scripts/release-check.js v0.1.1 && npm test
  git commit -am "Release 0.1.1" && git tag v0.1.1
  git push origin main && git push origin v0.1.1
  gh run watch --repo xo-lang/vscode
  ```

  A version publishes only once per registry: if a run fails after a
  publish step, fix the cause and release the next patch version.
- Secrets: `VSCE_PAT` (an Azure DevOps personal access token) and
  `OVSX_PAT` (an open-vsx.org access token for a member of the `xo-lang`
  namespace), set as the repository's GitHub Actions secrets
  (`gh secret set VSCE_PAT --repo xo-lang/vscode`, or Settings, Secrets
  and variables, Actions). Never in either repository, a commit, a file,
  or a log.
- Yearly renewal: Azure DevOps tokens last at most one year. Before
  `VSCE_PAT` expires, create a new token at dev.azure.com (User
  settings, Personal access tokens) with scope **Marketplace: Manage**
  and organization **All accessible organizations**, then replace the
  secret. A release that fails at the Marketplace step with a 401
  usually means it expired. Fallback without a token: `vsce publish
  --packagePath FILE --azure-credential` from a machine signed in with
  the Azure CLI (Microsoft Entra ID) as a member of the publisher.

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
