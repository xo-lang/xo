# 0139: The self hosted checker, compared through `xo typedump` in stages

Status: Accepted (2026-10-10). Stage 1 (names) implemented in
`selfhost/check/` (Xo), `internal/check/dump.go` (`Result.Dump`,
`DiagLines`), name records in `internal/check/info.go` (`records.names`,
written next to each resolution in decl.go, call.go, expr.go, stmt.go),
`TParam.Span` (types.go), `cmd/xo/typedump.go` (hidden `xo typedump`),
`internal/selfhost/check_test.go` (`TestSelfhostCheckerNames`,
`TestSelfhostCheckerXoTests`), `internal/selfhost/check_fuzz_test.go`
(`TestSelfhostCheckerNamesMutations`). Evidence:
`bench/results/selfhost-checker-2026-10-10.md`. Third step of decision
0130, after the lexer (0076) and the parser (0127).

## Context

The checker (internal/check, about 13,400 lines) is the largest
component so far and does many things at once: name resolution, type
inference, generics, effects, value semantics rules, patterns and
exhaustiveness, diagnostics with notes and fixes, and the habit
diagnostics of decision 0087. Porting it in one piece would leave
nothing to compare for months. The lexer and parser were compared
through a text dump both implementations print; the checker needs one
too, and one that can be compared a part at a time, because the Xo
checker will cover the parts in turn.

## Decision

**The dump.** `xo typedump [--section=S] [--each] [--bench=N] <dirs...>`
(hidden, like `astdump`) checks dirs as `xo check` does and prints, for
the root modules, in sections a test can cut apart:

```
== module <path> <dir>
-- names <file>
<line>:<col> <name> <object>
-- types <file>
<line>:<col>-<line>:<col> <node kind> <type>
-- sigs
<pos> <Type.name> <signature>[ uses <effects>]
== diags
{"code":...,"file":...,"line":...,"col":...,"end_line":...,"end_col":...,"message":...}
```

- An object is `local <let|var|param|var-param|recv|var-recv|predeclared>
  <pos>`, `fn|const|type <module path> <name> <pos>`, `tparam <name>
  <pos>`, or `module <path>`; a position is `file:line:col` of the
  declaring name, `-` when there is none (the `os` of a test). Lines are
  sorted by offset (names), by span (types), by declaration (sigs).
- names: what each identifier resolves to by name lookup, recorded by
  the Go checker where it resolves it (never inferred from the tree), so
  a resolution the checker skips has no line.
- types: `Info.TypeOf` of every checked expression, printed with
  `Type.String` after `Deep`.
- sigs: every function and method of the module: type parameters
  (implicit ones marked), receiver, parameters, result, error type, and
  the declared or inferred effects.
- diags: every diagnostic of the run as JSON with code, span, and
  message, sorted by position, code, and message (notes and fixes come
  with their stage).
- `--each` checks every dir as a run of its own (`=== check <dir>`
  before each), so one process covers the repository.

**The stages**, each compared on every module of the repository before
the next starts, through the sections named:

1. **Names** (names, name diagnostics; this decision): scopes, locals,
   parameters, receivers, type parameters (declared and implicit),
   top level declarations, imports (`use`, `testing` in test code),
   std and prelude names, module members, pattern paths up to the enum,
   duplicates, unused imports. Deferred, with no line on either side: a
   bare UpperCamel name used as a value or called that the module does
   not declare (it may be a variant of the expected enum: `Red`,
   `Some(x)`, `Int(x)`; the Go dump omits a prelude type called by
   name), struct fields, methods, statics, variants, named arguments,
   effects and effect arguments, loop labels.
2. **Types and inference** (types): expression types, unification,
   literals, expected types, conversions, Option and Result rules;
   fields, methods, variants, and named arguments resolve here.
3. **Generics** (types, sigs): type parameters, constraints, implicit
   receiver parameters, instantiation, XO0402.
4. **Effects and capabilities** (sigs, diags): `uses`, effect variables
   and elided effect arguments, inference of non pub effects, ceilings,
   real-time.
5. **Value semantics rules** (diags): mutation (XO0501, XO0502), lost
   updates (XO0520), aliasing (XO0503), moves.
6. **Patterns and exhaustiveness** (diags).
7. **Diagnostics with notes and fixes** (a full JSON section).
8. **Habits** (decision 0087): every habit diagnostic and fix.

**Comparison rules of stage 1** (`TestSelfhostCheckerNames`, all three
backends, every directory of the repository holding .xo files checked
alone): a run the Go checker accepts must give an identical names
section and no Xo diagnostic; a run whose errors are all name
diagnostics (XO0601 for an undefined lowercase name, type, module,
member, enum, or derivable interface, XO0603 duplicates, XO0604, XO0605,
XO0110 for imports) must give exactly those diagnostics. A run with a
`use go` or `use c` import is skipped (the Xo checker does not bind Go
packages or C headers); a parse error counts as a rejection. Mutations
of the repository's modules (a renamed identifier, a removed `use`, a
duplicated declaration) are compared the same way.

**The Xo checker** keeps the graph in tables: objects, modules, and
files are entries of Lists in one `Checker` value and refer to each
other by index (`-1` for none); scopes are a List of `Map[Bytes, Int]`;
the name records are a List with a map from `file:offset` to its entry.

## Consequences

- A change to name resolution in internal/check must be made in
  selfhost/check in the same change, or `go test -short ./...` fails.
- The Go checker carries a record per resolved identifier; it never
  influences what the checker accepts.
- Stage 1 found three compiler bugs, fixed with goldens (nested
  optional `??` on the Go backend; same named types of two modules and
  the evaluation order of an assignment on the native backends,
  decision 0140).

## Alternatives

- **One dump of everything, compared at the end**: nothing to compare
  until the whole checker exists.
- **Compare `xo check --json` only**: diagnostics say nothing about an
  accepted program, which is most of the repository.
- **Derive the names section from the tree** (walk identifiers, look
  them up): would test the dump, not the checker; the records are
  written where the checker resolves.
- **Pointers (references) for the Xo object graph**: Xo values have no
  identity; indices into tables are the idiom and cost nothing in
  clarity here.
