# 0438: did-you-mean notes for agents' Go and Rust habits, and `--backend=native`

Status: Proposed (Xo Lang 2, 2026-10-30; the lead's queue item from the
agent survey's item 2). Built the same day; the owner decides.

## Context

The agent survey of 2026-10-30 listed habits that cost agents a second
attempt because the diagnostic named the symptom, not the Xo form:

- `r.unwrap()` on a Result: the note talked about a missing value and
  did not lead with `?`; `r.unwrap_or(0)` on a Result had no note.
- `parse(s) + 1` with a Result operand: `XO0406` "operator `+` is not
  defined for `Result[Int, Str]` and `Int`" with no way forward.
- `os.args`: "`Os` has no field or method `args`".
- `let test = 1`: `XO0165` "expected a pattern, found keyword `test`".
- `List[fn(Int) uses go, fx]`: "`List` takes 1 type argument(s), found
  2" (the parser's `XO0170` covered the same habit only in parameters).
- `fn f(r: Int ! Str)`: `XO0160` "expected `,` or `)` in parameter list,
  found `!`".
- `xo build --backend=native`: rejected, though the docs call the
  default "the native backend".
- `names.sort()` as a statement: `XO0209` fired (it broke fair_pairs on
  main, 90821263e) but its note did not show the assignment.

Spec 10 (decision 0087) gives a habit a note and, when one rewrite keeps
the meaning, a fix, on the code that already describes the error; codes
never change meaning.

## Decision

Notes (and fixes where the rewrite is unambiguous) on the existing codes:

1. `XO0601` on `.unwrap()`/`.get()`/`.orElseThrow()` of a Result: "Xo
   propagates it with `?`, says why it cannot fail with
   `.or_fault("why")`, or handles it with `??`, `if let`, or `match`".
   `.unwrap_or(x)` (`.or`, `.get_or_else`, `.value_or`) on a Result: Xo
   writes `r ?? _ => x`, with that fix, parenthesized because an arm body
   runs to the end of the expression. `.expect(msg)` keeps its
   `.or_fault(msg)` fix.
2. `XO0406` for a binary operator with a Result operand: "the left (or
   right) side is a Result; add `?` to propagate its error, or
   `.or_fault("why")`".
3. `XO0601` on `os.args`, `os.argv`, `os.Args` (field or call): "did you
   mean `os.env.args()`? it leaves out the program name, so Go's
   `os.Args[1]` is `os.env.args()[0]`". No fix: a rewrite of the name
   alone shifts an index. The expression checks as `os.env.args()`.
   Env.args's doc comment says it excludes the program name.
4. `XO0165` and `XO0160` at a keyword that names things in other code
   (`test`, `type`, `in`, `as`, `match`, `use`, `select`, and the
   declaration keywords): "`test` is a keyword, so it cannot name a
   variable, parameter, or field; rename it, for example `is_test`".
   Statement keywords (`if`, `return`) get no note.
5. `XO0170` (several effects in a function type must be parenthesized)
   is also reported by the checker inside type arguments: when a type
   has more arguments than parameters and a function type with one
   unparenthesized effect is followed by a lowercase name, the name is
   read as a second effect, with the parenthesizing fix, and checking
   goes on as if it were written. The parser cannot do this, since only
   the arity tells `Cache[fn() uses net, fx]` (an effect argument of
   Cache) from the habit.
6. `XO0160` at `!` after a parameter's type, now a soft error: "`T ! E`
   is only a function's result; a parameter of a Result type writes
   `Result[T, E]`", with that fix; the parameter reads as the Result.
7. `--backend=native` names the default native backend: arm64 or amd64
   by the targets' architecture, else the host's (decision 0270's
   choice). It is the same as giving no --backend, except that it never
   falls to the Go backend for the Go-only flags (`--hybrid`, `xo run
   --record`), which then report that they need the Go backend.
8. `XO0209` on a method of a `var` binding gains the note "assign it
   back: `names = names.sort()`" beside its existing fix.

Each has a habit golden in cmd/xo/testdata/habits (TestHabitGolden),
which fails with the xo before the change; `--backend=native` is in
TestDefaultBackendAmd64.

## Consequences

- No code changes meaning; `XO0170`'s row in spec 10.6 names the checker
  too. `XO0160` at a parameter's `!` becomes a soft error, so the rest
  of the file is checked.
- `--backend=native` in scripts and docs is accepted from this xo on;
  an older xo rejects it.

## Alternatives

- New codes for each habit: rejected by spec 10 (a habit keeps the code
  that describes the error).
- A fix for `os.args`: rejected, the index shift changes meaning.
- `--backend=native` as an error naming arm64 and amd64: rejected;
  agents write it because the docs call the default the native backend.
