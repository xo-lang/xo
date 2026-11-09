# tree-sitter-xo

Last verified: 2026-11-09

A tree-sitter grammar for Xo, written from spec/core.md by Xo Lang 3 (owner approval 2026-11-08, name confirmed by the owner).

- Location: `~/godev/claude-space/xolang/tree-sitter-xo`, public since 2026-11-08 at https://github.com/xo-lang/tree-sitter-xo
  (main, no rulesets, like xo-lang/vscode; gitleaks and the scrub list ran before the push, commit emails are the github@
  address of the other public repos). Published 2026-11-09 by the owner: npm `@xo-lang/tree-sitter-xo` (scoped: npm refused
  the plain name as too close to tree-sitter-go) and crates.io `tree-sitter-xo`, 0.1.0 (ABI 14, tag v0.1.0) and 0.1.1
  (ABI 15, tree-sitter-cli 0.27.2, tag v0.1.1). Every outside registration and editor PR, with what it waits on and
  the pins to bump: [outside-registrations.md](outside-registrations.md).
- Contents: `grammar.js`, the external scanner `src/scanner.c` (Go's newline rule, raw strings), generated `src/`, queries
  (highlights, locals, indents, folds, injections), corpus tests (`test/corpus`) and highlight assertions
  (`test/highlight`), bindings (Node, Rust, C, Go, Python, Swift), README with Neovim, Helix and Zed setup, MIT license.
- Check: `npx tree-sitter test`, and `scripts/parse-repo.sh <xo checkout>`: every `.xo` file except test/errorcheck and
  testdata/habits (invalid on purpose) parses with no ERROR node (2488 of 2488 on 2026-11-08).
- Keep in step with the language: a syntax change in spec/core.md needs a grammar change and a corpus case; run the repo
  parse script after a release.
