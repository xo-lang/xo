# 0090: Method syntax: keep Go style receivers (Appendix B question 1)

Status: Proposed (2026-10-09, spec audit; awaiting the owner)

## Context

Spec Appendix B question 1 asks whether methods are declared with Go style
receivers (`fn (u: User) display_name() -> Str`, current, core 3.4) or
inside `impl` blocks (`impl User { fn display_name(self) -> Str }`, Rust,
Swift extensions). The answer fixes surface syntax for the 1.0
compatibility promise, so it has to be settled before 1.0 (decision 0089).

Facts that constrain the choice:

- Interfaces are satisfied structurally and implicitly (core 2.4). There is
  no `impl Trait for T`, so an `impl` block would only group methods; it
  would carry no conformance information.
- Methods may be declared only in the module of their type (core 3.4,
  `XO0606`), so both forms keep every method of a type in one module.
- A `var` receiver (`fn (var a: Account) deposit(...)`) marks in out
  mutation per method (core 3.4, 5.2). In an `impl` block this needs a per
  method marker anyway (`fn deposit(var self, ...)`), as Rust's `&mut self`.
- Agents are the main writers (core 0).

## Options

1. **Go style receivers (current).** One declaration form for functions and
   methods; each method stands alone, so a method can be read, moved,
   renamed (`xo edit rename`), and diffed without its neighbours.
2. **`impl` blocks.** Groups methods; the receiver name is `self`
   everywhere; one more nesting level and one more keyword (`impl` is not
   among the 28).
3. **Both.** Rejected outright by design goal 1 ("one obvious way").

## Evidence

- Paper tests (`bench/paper/v0.1/01-crud-api/NOTES.md`,
  `v0.2/03-retry-client/NOTES.md`, `v0.3/08-notes-service/NOTES.md`): in 3
  of the blind write-ups the agent first reached for a Rust `impl` (for
  `Display` or for a status method) and then wrote a receiver method; no
  write-up reported a receiver mistake or asked for grouping.
- The repository's Xo corpus (bench solutions, paper solutions, examples;
  108 files) has 124 receiver methods and no request for grouping in any
  results file.
- Habit diagnostics (decision 0087,
  `bench/results/habit-diagnostics-2026-10-09.md`): an `impl` block is
  already reported as `XO0177` with a machine fix that rewrites it into top
  level receiver methods; of 149 habit cases only the `impl` whose
  function takes no `self` lacks a fix (it is a static function, which Xo
  writes as a module function). So Rust and Swift habits cost one
  `xo fix` round, not a language change.
- Tooling: `xo edit rename`, `add-field`, `change-sig`, `xo query def`,
  and the self hosted lexer (`selfhost/lexer`, 1394 lines, decision 0076)
  all work on standalone receiver declarations; an `impl` form would add a
  second declaration shape to every one of them.

## Recommendation

Keep Go style receivers (option 1) and close the question. Keep the
`XO0177` habit fix for `impl` blocks as the migration path for Rust and
Swift writers.

## Consequences

- Appendix B question 1 is removed from the open list once accepted;
  `impl` stays a non keyword (usable as an identifier), and the parser
  keeps recognizing it only to report the habit.
- No code change.

## Alternatives

- `impl` blocks: rejected for the reasons above (no conformance meaning
  with structural interfaces, a second declaration shape for tools,
  per method `var self` still needed).
- Allow both: violates "one obvious way".
