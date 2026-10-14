# 0219: Completion in xo lsp

Status: Proposed (stub, 2026-10-13); being implemented on a branch.

`xo lsp` answers no `textDocument/completion` (decision 0114: "No
completion or signature help yet"), so editors fall back to word
matching. This decision adds completion (members after `.`, package
members after `pkg.`, names in scope, keywords) and signature help,
modeled on gopls: the parser's recovery plus the checker's types at the
cursor, ranked by the expected type where known.
