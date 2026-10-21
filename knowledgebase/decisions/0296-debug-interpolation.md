# 0296: `${x:?}` interpolates the Debug text

Status: Accepted (owner, 2026-10-20, through the lead)

## Context

An interpolation `${x}` needs `Display` (core 1.6). `Result`, `List`,
`Map`, `Option`, and most structs have no Display, so formatting one for
a log line, a test failure, or a quick look meant `${x.debug()}`. Agents
wrote that form often and also reached for Rust's `{:?}` and Python's
`{x!r}`, which Xo rejected as foreign syntax with no fix. The lead took
`${x:?}` to the owner as a language question in the 2026-10-20 poll.

## Decision

- Grammar: an interpolation is `"${" Expr [ ":?" ] "}"`. `:?` follows the
  whole expression, so `${a + b:?}` formats the sum. It is the only format
  spec; any other text after the expression (`${x:.2}`) is `XO0160`, as
  before.
- Meaning: `${x:?}` is `x.debug()`, the Debug text, for any type (every
  type implements Debug, std.md 1). If a type has none, the error is
  `XO0404`: "`T` does not implement `Debug`, so `:?` cannot format it".
- Implementation: parser StringPart.debug (the interpolation's
  sub-parser accepts a trailing `:?`); the checker requires Debug instead
  of Display; the SSA builder appends the Debug text, as `.debug()` does,
  which covers arm64, amd64, LLVM, and wasm; the Go generator calls
  `rt.Debug`. defhash and the AST dumps record the flag only when it is
  set, so existing hashes and dumps do not change. The formatter prints
  string literals from source and needs nothing. The VS Code grammar
  scopes `:?` as a format modifier.
- Habits: Rust's `format!("{:?}", a)` and `{name:?}`, and Python's
  `f"{x!r}"`, now recover as `${a:?}` with a machine fix.
- `xo vet` notes `${x.debug()}` (`XO0535`) with a fix to `${x:?}`, for
  the prelude's Debug only (a type's own `debug` method is left alone).
- Std code may use `:?` only after the next seed (an older seed cannot
  parse it, gotchas/seed-older-than-intrinsics.md's rule).

## Consequences

- Logging and test messages lose a call: `log.info("got ${r:?}")`.
- A program written with `:?` does not build with a compiler older than
  this change; the language is pre 1.0 and versions move together.

## Alternatives

- Rust's `{x:?}` inside a `${}`-free string: a second interpolation form.
- More format specs (`:.2`, width, padding): no agreement on the set, and
  each needs its own rules per type; `:?` covers the asked case.
- `${x!r}` or `${debug(x)}`: `:?` is what agents already write from Rust.
