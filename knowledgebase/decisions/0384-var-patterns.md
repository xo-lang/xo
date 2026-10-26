# 0384: `var` with a tuple or list pattern (`var (a, b) = f()`)

Status: Accepted (owner, 2026-10-26: "Option 2 approved"). Built by Xo
Lang 2 the same day. Error `XO0179`, added first to replace a misleading
parse error, is retired and stays reserved.

## Context

Spec 3.1 and 3.5 bind a pattern with `let` only (`let (a, b) = pair`).
`var` takes one name. An agent poll (2026-10-26) asked whether
`var (a, b) = f()` is allowed. On main it was a parse error,
"expected a variable name, found `(`", followed by one "name `a` not
found" (XO0601) for every later use of each name. Those follow-on errors
are wrong whatever the answer: the names were declared.

Now (this change): `var` followed by `(` or `[` is error `XO0179`. Its
fix writes `let`, and the parser reads the statement as that `let`, so
the only other error left is a true one: an assignment to one of the
names is `XO0502` (an immutable binding).

What code does today when it needs a destructured value to change
(measured 2026-10-26 over compiler/ and std/, 2,244 `let (...) =`
statements): 49 of them are followed within three lines by
`var x = x0` copying one of the names into a `var`, for example
compiler/arm64/calls.xo:364-365:

```
let (b0, ok) = g.offset_base(l, i, es)
var b = b0
```

Other languages:

- Go: `a, b := f()` declares every name mutable (Go has no immutable
  locals).
- Swift and Kotlin: `var (a, b) = f()` is allowed; every name is
  mutable.
- Rust: mutability is per binding, `let (mut a, b) = f()`.
- TypeScript: `let [a, b] = f()` (mutable) and `const [a, b] = f()`.

## Decision

The owner chose option 2 (2026-10-26): `var PATTERN = e` takes the
irrefutable patterns `let` takes (tuples, structs by field, lists whose
shape is known), and every name it binds is mutable. A single name stays
`var x = e`. The options as proposed:

1. **Keep `let` only** (today, plus `XO0179`). No language change, and
   a pattern's names are always immutable. Code that needs one of them
   to change keeps the `let (a0, b) = ...` then `var a = a0` copy.
2. **Allow `var PATTERN = e`**: the same irrefutable patterns as `let`
   (tuples, lists of known shape, structs by field), and every name it
   binds is mutable. `var` stays one keyword with one meaning, "these
   names change", which is what Swift and Kotlin do and what Go and
   TypeScript users write by habit.
3. **Per binding `var` inside a `let` pattern**, Rust's
   `let (mut a, b)`: `let (var a, b) = f()`. Exact, but a second place
   for `var`, inside patterns, which `match` and `if let` would then be
   asked to take too.

Recommendation: **option 2**. It removes the 49 copy statements and the
`x0` names they need, it is the form agents write first (the poll), and
it adds no new syntax: `var` takes what `let` takes. The cost is that a
name the pattern binds but never changes is mutable for nothing. Xo has
no warning for an unchanged `var` today (a single-name `var x = 1` that
is never written is accepted), so option 2 does not make that worse.
`XO0179` would be removed (a code is never reused: it stays reserved,
spec 10).

Option 1 is the safe default if the owner prefers fewer ways to bind:
the diagnostic already gives the rewrite.

## Consequences

As built (option 2):

- The parser reads `var` followed by `(`, `[`, or a type name and `{`
  (or `.`) as a pattern: `Stmt.LetStmt` carries `is_var`, so every tool
  that walks statements sees one form (the formatter prints `var`, the
  definition hash and the AST JSON tell them apart).
- The checker declares the pattern's names as `var` locals
  (Checker.pat_local); every backend already assigns to `var` locals
  the same way, so no backend changed. The golden
  internal/llvm/testdata/golden/var_patterns runs on each backend.
- A `let` pattern's name that is assigned is `XO0502` with a fix that
  writes `var` for the whole pattern, as it already did for one name.
- The compiler's own code must not use the form until a seed that
  accepts it exists (the seed rule); the 49 copy sites are simplified
  in a later change.

As proposed:

- Option 1: nothing more to do; `XO0179` stays.
- Option 2: parser (`compiler/parser/stmt.xo`: the `var` branch parses
  a pattern), the checker's binding of pattern names marks them
  mutable, the backends' assignment to pattern bindings (they already
  handle `let` patterns and `var` names), spec 3.1 and 3.5, tests on
  every backend (a golden that destructures into `var` and assigns),
  and `XO0179` retired. The 49 copy sites can then be simplified, as a
  separate change.
- Option 3: as option 2 plus pattern grammar for `var NAME` inside
  patterns and a decision on whether `match` and `if let` take it.

## Alternatives

Listed above as options 1 and 3. Also considered: accepting
`var (a, b)` silently as `let` (rejected: the names would be immutable
while the code says `var`), and making the parser error alone clearer
without the recovery (rejected: the follow-on "name not found" errors
were the misleading part).
