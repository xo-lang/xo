# 0113: Editor support starts with a TextMate grammar in the repo

Status: Accepted (2026-10-10, by the owner)

## Context

`.xo` files had no highlighting in any editor. The owner uses VS Code.
VS Code colors files from TextMate grammars, and can add semantic tokens
from a language server on top. Xo has no language server; `xo check`,
`xo query`, and `xo mcp` already expose the compiler's knowledge.

## Decision

Ship a VS Code extension at `editors/vscode/` with a TextMate grammar
(`source.xo`) and a language configuration, versioned with the compiler.
It is installed locally by symlink; it is not published to the
Marketplace for now. A Go test keeps the grammar's keyword set equal to
`internal/token`, and `npm test` runs tokenization assertions.

## Consequences

- Highlighting is lexical: names are colored by naming convention
  (`UpperCamel` types, `UPPER_SNAKE` constants) and position, not by
  resolution. Enum variants color as types.
- Lexical changes in the spec must update the grammar in the same change.
- Node is needed only for the grammar's tokenization tests, not for the
  compiler build.
- The grammar can be reused by other TextMate based tools (GitHub
  Linguist, Shiki, Sublime) later.

## Alternatives

- `files.associations` mapping `.xo` to Go: no work, but `${}`,
  `#{}`, raw strings, `let`, `fn`, `match`, and contract clauses color
  wrongly.
- Language server first (`xo lsp` with semantic tokens): richer, but
  gives nothing until it starts and still benefits from a grammar
  underneath. Planned as the next step, layered on this grammar.
- Tree-sitter grammar: VS Code does not use tree-sitter for
  highlighting.
