# 0087: Habit diagnostics for code written with other languages' syntax

Status: Accepted (2026-10-09)

## Context

Codex, writing Xo for the first time from the spec, passed `xo check` on
the first try in 3 of 10 tasks against 9 of 10 for Claude, and all 7
failures were at the checker, from habits of other languages that its own
notes name: Rust style bounds (`K: Hash + Eq`), commas between match
arms, `var` at a call site, `use` after declarations, and an effect
argument left out of a parameter type that stayed abstract
(bench/results/codex-2026-10-09.md). The diagnostics it got were generic
(`XO0160 expected ,`), cascaded (one `+` in a bound list gave 96 errors in
the cache task, from `xo check` losing the type's parameters), or offered
wrong machine fixes (`fx` replaced by `Fs`, `Vec` by `Eq`, `string` by
`Stdio`, a loop counter `t` by `it`), which `xo fix` applies blindly. An
agent new to Xo writes Go, Rust, TypeScript, Python, Swift, or Kotlin
forms first; the checker is where it learns Xo, so the checker should
teach.

## Decision

1. **One habit, one diagnostic, naming the habit and the Xo form.** The
   message says where the form comes from and what Xo writes instead
   ("`x := v` is Go syntax; Xo binds with `let x = v`, or `var x = v`
   when it is reassigned later"). The parser and checker then continue as
   if the Xo form had been written (the parser builds the Xo AST; the
   checker declares the name, types the call as its Xo equivalent), so
   nothing downstream cascades.
2. **A machine fix only when one rewrite keeps the meaning.** Examples
   with fixes: `func`/`fun`/`def`/`function` to `fn`, `T: Hash + Eq` to
   `T: Hash` (`Eq` and `Debug` are automatic), `var x` and `&x` at a call
   site to `x`, `x := v` to `let` or `var` (by a scan for later
   assignments), Go's `x, err := f()` plus `if err != nil { return ... }`
   to `let x = f()?`, `for i := 0; i < n; i++` to `for i in 0..n`, Go
   `for k, v := range m` to the form for the type of `m` (decided in the
   checker), `c ? a : b` to `if c { a } else { b }`, `switch`/`when` to
   `match` (when no case falls through or is empty), `len(x)`,
   `append(xs, v)`, `range(n)`, `print` in a function with an `Os` or
   `Stdio` parameter, Rust `impl` blocks to top level methods with a
   `self` receiver, and an elided effect argument (the fix removes the
   closure's own `uses` list so its effects are inferred). No fix where
   the Xo form needs a decision the program does not state: `x!!` and
   `.unwrap()` need a reason for `or_fault`, `try`/`catch` needs a choice
   between `?` and `match`, a bare `print` has no capability in scope,
   `map.get_or_fault(k)` needs its reason. Those name the form and stop.
3. **Code allocation.** Syntax that the grammar would reject only with a
   generic code (`XO0160`, `XO0168`, `XO0172`, `XO0175`) gets one new
   code, `XO0177` (syntax from another language), whose `xo explain`
   lists the habits. Where a specific code already describes the error,
   the habit keeps that code and adds the note and fix: `XO0601` (names,
   calls, types, methods: `nil`, `null`, `this`, `len`, `String`, `Vec`,
   `.unwrap()`, `.length`, camelCase methods, Python's declaration by
   assignment), `XO0169` (arm commas), `XO0173` (`use` after declarations,
   now with a fix that moves it), `XO0701` (elided effect argument), `XO0408`
   (TypeScript `const`), `XO0110` (Kotlin `"$name"`), `XO0101`, `XO0502`,
   `XO0150`, `XO0152`, `XO0153`. No existing code changes meaning; one
   code for all syntax habits keeps the code space small and lets an agent
   look up every habit with one `xo explain`.
4. **Edit distance guesses become fixes only when clear**: a unique
   closest candidate at most half the name's length away, names of three
   or more characters. The note still lists the candidates.
5. **No accepted program is rejected.** Every habit form was an error
   before; forms that could be valid Xo (`pass`, `not`, `await` as
   variable names, a `?` before a map entry) are left alone or handled in
   the checker after name resolution. `xo check` over all 361 directories
   with `.xo` files in the repository reports the same codes at the same
   places before and after.
6. **The lexer does not change.** It must stay byte for byte equal to its
   self hosted port (selfhost/lexer, decision 0076), so lexical habits
   (`#` and `/* */` comments, `#[derive]`, backtick templates, Swift's
   `\(x)`, `'text'`) are handled by a token pass in the parser that
   rewrites the lexer's per character errors into one diagnostic.

## Consequences

- On Codex's 7 failing first runs, reconstructed from its final code and
  notes, `xo check` now gives an exact fix for every error in 5 and
  `xo fix` makes those 5 green (0 of 7 before); 2 remain, one needing a
  reason string and one hitting std gaps (`Duration.parse`, JSON for
  tuples) (bench/results/habit-diagnostics-2026-10-09.md).
- 149 golden cases (internal/fix/testdata/habits) hold the input, the
  JSON diagnostics, and the fixed program, which must pass `xo check`;
  TestHabitsReported and TestHabitsNotReported keep each habit at one
  diagnostic and near misses at none.
- The parser carries heuristics that must not fire on valid Xo; each one
  needs a negative test when added.
- Messages name languages; they are guidance for agents, not a style
  judgment, and may grow as new habits show up in runs.

## Alternatives

- A separate lint pass after a failed parse: would not prevent the
  cascades, which come from parser recovery.
- One new code per habit: dozens of codes with one example each; agents
  and docs would need a table to find them anyway.
- Accepting some foreign forms as Xo (`:=`, `elif`, both `&&` and `and`):
  two ways to write one thing costs the single canonical form agents rely
  on when reading code (core 0, goal 1: no guessing), and `xo fmt` would
  have to pick one anyway.
- Fixes that insert a hole (`???`) for the missing decision: the program
  then checks (holes are warnings) but faults at run time, which hides
  the decision rather than asking for it.
