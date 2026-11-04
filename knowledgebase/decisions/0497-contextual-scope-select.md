# 0497: `scope` becomes a contextual word (`select` stays a keyword)

Status: Accepted (owner, 2026-11-04): option (b), `scope` only: `scope` becomes a contextual word and `select` stays a keyword. Proposed by Xo Lang 2 the same day from the AWS agent's feedback log; the decision section below is the proposal as written, (b) its `scope` half.

## Context

Core 1.4 lists 28 keywords; `scope` and `select` are two of them. The
AWS agent's feedback log (2026-11-04, "Common words are keywords")
hit both: SigV4's credential scope is naturally `scope`, and `select`
is an AWS member name (with `test`, `type`, `match`) that its code
generator had to rename. XO0101's note says what to write
(`scope_name`), so the cost is a rename, but it is paid by every
generated binding and every hand port of an API that uses the word.
Xo already makes position-bound words contextual (`derive`, `with`,
`from`, `after`, `is`, `bound`, `realtime`; decision 0015 dropped
`timeout` for the same collision with parameters).

Both words begin an expression only in shapes no identifier can take:

- `scope NAME {`: two identifiers side by side are never an
  expression, so `scope` followed on the same line by an identifier and
  `{` is the scope expression, and anything else is a name.
- `select {`: an identifier followed by `{` is an expression only where
  struct literals are allowed, and a struct literal needs an UpperCamel
  type name (XO0101), so lowercase `select {` is never a literal. The one
  overlap is a header that bans unparenthesized literals (`if`,
  `while`, `for`, `match`, `requires`, `ensures`; core 1.4's last
  paragraph), where `if select {` could be a Bool variable `select`
  followed by the `if`'s block.

## Decision (proposed)

`scope` and `select` leave the keyword list (28 to 26) and join the
contextual words:

- `scope` is the scope expression when followed on the same line by an
  identifier and `{`; anywhere else it is an identifier.
- `select` is the select expression when followed on the same line by
  `{`, except directly as the condition of an `if`, `while`, `for` or
  `match` header (core's no-literal positions), where it is the
  identifier; a select there is written in parentheses, as a struct
  literal is.
- The lexer emits them as identifiers; the parser checks the word at
  expression start (as it does `derive`, `parser.xo is_word`), so the
  AST, the formatter, the checker and the backends see the same
  ScopeExpr and SelectExpr as today.
- Existing programs keep their meaning: every use of `scope` or `select`
  today is one of the expression shapes.

## Consequences

- Fields, parameters, locals, functions and methods may be named
  `scope` or `select`; generated bindings stop renaming them.
- The Go backend must emit a Go-safe name for an Xo identifier `select`
  (a Go keyword), as for any other Go keyword that is a valid Xo
  identifier (`go`, `chan`, `default`, `range`, `func`, `map`): check
  compiler/gen's identifier escaping covers it, with a golden.
- Editor highlighting (the TextMate grammar, `xo lsp` semantic tokens)
  colors the words as keywords only in the expression position.
- A reader sees `scope` both as a keyword and as a name in one file; the
  shapes are distinct enough (`scope t {` against `scope.region`) that
  this is the same cost as `with` and `derive` today.
- The ambiguity rule for `select` in a condition is one more thing to
  learn; it follows the existing struct literal rule, and the parser's
  error for `if select { ... }` with no `select` variable in scope can
  say to parenthesize.

## Alternatives

- (a) Keep both keywords; XO0101's rename note stays the answer (no
  change; the generator keeps its rename table).
- (b) `scope` only: it carries the realistic collision (credential
  scope, OAuth scope, a log scope), and its rule has no exception;
  `select` stays a keyword.
- (c) Also `test`, `type` and `match` (the generator's other renames):
  `type` and `match` begin declarations and statements at positions
  where an identifier also fits (`match x {` against a call `match(x)`),
  so their rules would need lookahead past a whole expression; not
  proposed.

Recommendation: the decision as written (both words). If the owner wants
the smallest step, (b) first: it removes the commonest collision with no
special case.
