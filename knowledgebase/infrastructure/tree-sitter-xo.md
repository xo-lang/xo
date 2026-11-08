# tree-sitter-xo

Last verified: 2026-11-08

A tree-sitter grammar for Xo, written from spec/core.md by Xo Lang 3 (owner approval 2026-11-08, name confirmed by the owner).

- Location: `~/godev/claude-space/xolang/tree-sitter-xo`, a local git repository. Nothing is published: no GitHub repository, npm
  package or crate until the owner approves (ask the lead first). The intended home is xo-lang/tree-sitter-xo.
- Contents: `grammar.js`, the external scanner `src/scanner.c` (Go's newline rule, raw strings), generated `src/`, queries
  (highlights, locals, indents, folds, injections), corpus tests (`test/corpus`) and highlight assertions
  (`test/highlight`), bindings (Node, Rust, C, Go, Python, Swift), README with Neovim, Helix and Zed setup, MIT license.
- Check: `npx tree-sitter test`, and `scripts/parse-repo.sh <xo checkout>`: every `.xo` file except test/errorcheck and
  testdata/habits (invalid on purpose) parses with no ERROR node (2488 of 2488 on 2026-11-08).
- Keep in step with the language: a syntax change in spec/core.md needs a grammar change and a corpus case; run the repo
  parse script after a release.
