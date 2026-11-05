# 0516: the VS Code extension lives in xo-lang/vscode

Status: Accepted (owner, 2026-11-05)

## Context

The VS Code extension lived in editors/vscode since decisions 0113 and
0114: a TextMate grammar, a Markdown injection, and an `xo lsp` client,
with a Go test (grammar_test.go) that read the lexer's keywords from
compiler/lexer/token.xo of the same checkout. It found xo in a checkout
first (bin/xo, then .tools/bin/xo) and on PATH last, which suits working
on Xo and not a user who installed a release. With `xo debug --dap`
(decisions 0253, 0504) an editor can debug Xo programs, which the
extension did not offer. The owner decided on 2026-11-05 that the
extension moves to its own repository.

## Decision

- The extension is the repository xo-lang/vscode (one extension,
  Marketplace id `xo-lang.xo`, "Xo"), with editors/vscode's history
  (`git filter-repo --subdirectory-filter editors/vscode` of a clone at
  origin/main, 11 commits), its files at the root. editors/vscode and its
  Go test leave this repository.
- It contributes a debugger, type `xo`, that runs `xo debug --dap`:
  launch (`program`: an executable, or a `.xo` file or package directory
  that the extension builds with `xo build` first; `args`, `cwd`) and
  attach (`processId`, `program`); F5 on an open `.xo` file debugs its
  package.
- It finds xo as an installed user has it: the `xo.path` setting, then
  `xo` on PATH, then a checkout's `bin/xo` or `.tools/bin/xo` in or above
  a workspace folder.
- The grammar check runs there with no Xo checkout: `npm run
  check:grammar` reads the keywords from the newest Xo release's
  `compiler/lexer/token.xo` at its tag on GitHub (the API's latest
  release), or a local file with `--lexer`; a release's lexer is what
  users' xo accepts, so the grammar follows releases.
- Tests there: Node unit tests (the xo lookup, the debug configurations,
  the manifest), the grammar's tokenization tests, a DAP end to end test
  against an xo with `debug --dap` (`XO_TEST_XO`), and a run of the
  packaged `.vsix` in a VS Code of its own (`npm run test:vscode`:
  highlighting, `xo lsp` hover and semantic tokens, a breakpoint).
- Nothing is pushed to GitHub, no workflow runs and nothing is published
  to the Marketplace until the owner does it.

## Consequences

- A change to Xo's lexical structure no longer fails `go test -short`
  here when the grammar lags; the grammar is updated in xo-lang/vscode
  after the release that has the change, and its check fails until then
  (its CI also runs weekly, for a new release).
- Working on Xo with an older xo on PATH, set `xo.path` to the checkout's
  `bin/xo` (infrastructure/editors.md).

## Alternatives

- Keep the extension in this repository: one place for the grammar and
  the lexer, but the extension's release cycle (Marketplace versions) and
  Node tooling would sit in the compiler's repository, and the owner chose
  its own.
- A debug extension separate from the language extension: two installs
  for one language; the owner chose one extension.
- A grammar check through a new `xo` command printing the keywords: it
  would only work from the first release that has it; reading the
  release's lexer source works for every release.
