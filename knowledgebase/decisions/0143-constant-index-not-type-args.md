# 0143: A constant indexed by a lowercase name is an index

Status: Accepted, implemented (2026-10-10). Spec: core 15 (parsing rules,
type arguments vs. indexing). Code: `internal/parser/parser.go`
(`typeArgsAhead`, `isConstName`), `selfhost/parser/expr.xo`
(`type_args_ahead`, `is_const_name`). Tests: `internal/parser`
(`NAMES[i]`, `KIND_NAMES[k].len()`, `TABLE[i](x)`, `Runner[fx].new()`),
`internal/gen/testdata/golden/const_index` (all three backends). Found by
the self hosted checker, stage 2 (decision 0141).

## Context

`x[...]` is type arguments or an index, decided by shape (gotcha
type-args-vs-index). Since effect arguments were allowed (spec 0.4), a
lowercase `name` in the brackets counts as a type element, and a base
that starts with an upper case letter counts as a type name. So
`KIND_NAMES[k]`, a constant list indexed by a local, parsed as the type
`KIND_NAMES` with the effect argument `k`, and the checker rejected it
("type arguments are only allowed on generic functions and types"). The
workaround was a local copy (`let names = KIND_NAMES; names[k]`).

## Decision

When the base is an identifier spelled like a constant (upper case
letters, digits, and underscores: `NAMES`, `KIND_NAMES`, `N`) and the
brackets hold no element that looks like a type (only lowercase names
or `name.sub`), `x[...]` is an index. Everything else keeps its reading:
`Cache[Int, Item, net.dial]`, `Runner[fx].new()`, and `m[Red]` are as
before.

## Consequences

- Constant tables are indexed directly, as other languages allow.
- A type spelled in capitals whose only type arguments are effect
  arguments (`IO[fx]`) is now read as an index; types are UpperCamel
  (core 1.5), so such a name is already against the convention.

## Alternatives

- **Decide in the checker** (reparse a TypeArgsExpr whose base is a
  constant as an index): the parsers would still disagree with what the
  programmer wrote, and the formatter and the AST tools would see type
  arguments.
- **Leave it and document the workaround**: a trap every table driven
  program meets.
