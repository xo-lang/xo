# 0141: The self hosted checker, compared through `xo typedump` in stages

Status: Accepted (2026-10-10). Stage 1 (names), stage 2 (types and
inference), stage 3 (generics), stage 4 (effects and real-time, but
dependency ceilings), the parse habits (pulled ahead of stage 8, in
selfhost/parser), stage 5 (value semantics), and stage 6 (patterns and
exhaustiveness) implemented in `selfhost/check/` (Xo: decl.xo, types.xo,
infer.xo, effects.xo, expr.xo, call.xo, stmt.xo, pattern.xo, habits.xo,
comparable.xo, realtime.xo, flow.xo, exhaust.xo, the sigs section in
dump.xo),
`internal/check/dump.go` (`Result.Dump`, `DiagLines`), name records in
`internal/check/info.go` (`records.names`, `records.field`,
`records.vfield`, written next to each resolution in decl.go, call.go,
expr.go, stmt.go, pattern.go), `TParam.Span` (types.go),
`VFieldObj.Span` (objects.go), `cmd/xo/typedump.go` (hidden `xo
typedump`), `internal/selfhost/check_test.go` (`TestSelfhostChecker`,
`TestSelfhostCheckerXoTests`), `internal/selfhost/check_fuzz_test.go`
(`TestSelfhostCheckerMutations`). Evidence:
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
  <pos>`, `fn|const|type <module path> <name> <pos>` (a method, static,
  or interface method is `fn <module path> <Type>.<name> <pos>`),
  `tparam <name> <pos>`, `module <path>`, `field <module path>
  <Type>.<field> <pos>` (a variant's field: `<Enum>.<Variant>.<field>`),
  `variant <module path> <Enum>.<Variant> <pos>`, or `param <module path>
  <fn>.<param> <pos>` (the parameter a named argument names); a position
  is `file:line:col` of the declaring name, `-` when there is none (the
  `os` of a test). Lines are sorted by offset (names), by span (types),
  by declaration (sigs).
- names: what each identifier resolves to by name lookup and, from stage
  2, the field, method, static, variant, or parameter a selector, struct
  literal or `with` field, call, pattern, or named argument names,
  recorded by the Go checker where it resolves it (never inferred from
  the tree), so a resolution the checker skips has no line.
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
   fields, methods, variants, and named arguments resolve here. The
   instantiation of generic functions and types at calls, constraint
   checks (XO0404), and the effect sets that function types print (`fn()
   uses net`) and effect arguments (`Cache[K, V, E, net.dial]`) are
   needed for the types of non generic code and are part of this stage;
   XO0402, the sigs section, and checking effects belong to stages 3 and
   4.
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

**Comparison rules of stage 2** (`TestSelfhostChecker`, the same runs
and backends; they replace stage 1's): a run the Go checker accepts must
give identical names and types sections and no Xo diagnostic; a run
whose errors are all diagnostics of stages 1 and 2 must give exactly
those diagnostics. The stage 2 diagnostics (`stageDiag` in check_test.go)
are XO0202 to XO0206, XO0302, XO0304, XO0401, XO0403, XO0405 to XO0410,
XO0601 to XO0607, XO0702, XO0801, XO0803, XO0110 on imports, XO0201 on
discarded expressions, XO0404 for a value or type argument that does not
implement an interface and for the fields of a derive, and XO0502 on
assignment targets. Left to later stages:
XO0110 and XO0201 on bindings (unused), XO0402, XO0701, XO0501 to XO0520
but XO0502 on targets, XO0301, XO0303, XO0404 on comparability, XO0101,
and the parse habits (XO0177). `TestSelfhostCheckerMutations` adds type
mutations (a literal of another type, a `?` removed, a field or method
name replaced) to the name mutations.

**Comparison rules of stage 3** (2026-10-10; `TestSelfhostChecker`, the
same runs and backends; they replace stage 2's): a run the Go checker
accepts must give identical names, types, and sigs sections, the sigs
compared without their effects (`compareSigEffects` false: the last `
uses ...` of each sigs line is dropped on both sides), and no Xo
diagnostic; a run whose errors are all diagnostics of stages 1 to 3
must give exactly those diagnostics. Stage 3 adds XO0402 and every
XO0404 (comparability of keys, set elements, and `derive Eq` or `Hash`
included) to the stage 2 list. The mutations add generics edits (a type
argument removed from a written list, a constraint dropped from a type
parameter).

**Comparison rules of stage 4** (2026-10-10; they replace stage 3's):
the sigs section is compared with its effects (`compareSigEffects`
true), declared and inferred, and the staged diagnostics add XO0701 and
XO1401 to XO1405. XO1309 (effect ceilings of dependencies) is not
staged: no repository run requires a dependency, and the Xo checker
does not load the module cache. The mutations add effects edits (a
`uses` clause removed, or one added to a function declaration: an
effect label, or `realtime`, which applies core 6.5 to arbitrary code),
and `XO_SELFHOST_BACKEND` runs them on a native build. The test also
logs, without failing, how many runs rejected for other reasons still
get exactly the staged part of Go's diagnostics.

**Comparison rules of the parse habits** (2026-10-10, pulled ahead of
stage 8 because they were the only errors of 99 of the 143 runs not
compared; they extend stage 4's): the Xo parser reports the habits and
soft errors of the Go parser with the same messages and spans and
recovers the same tree (decision 0127), and the Xo checker checks that
tree: a hard parse error makes the run unsupported, and the parser's
diagnostics join the run's. A diagnostic of a run is staged when it is
one of the stage 1 to 4 list or one the Go parser gives for its file
(`parseDiag` in check_test.go parses the file again and compares the
diagnostic line), so a run whose errors are habits and diagnostics of
stages 1 to 4 must give exactly those diagnostics. The habits the Go
checker reports (`for k, v := range x`, `String::from`, time layouts,
`len(x)`: internal/check habits.go) stay in stage 8. The mutations add
habit edits (habitEdits in check_fuzz_test.go: `x := v`, `val`, `let
mut`, `func`/`def`/`function`, `and`/`or`, `x++`, `?:`, `...`, `elif`,
`loop`, `throw`, `#` comments, Go parameters, `): T`, `===`, `[]T`,
`map[K]V`, `List<T>`).

**Comparison rules of stage 5** (2026-10-10, value semantics; they
extend the rules above): the staged diagnostics add XO0501, every
XO0502 (not only on assignment targets), XO0503, XO0510, XO0520, the
XO0804 warning (split critical sections, ported with flow.go), and every
XO0110 and XO0201 (unused bindings and Results bound and never read).
Since a run the Go checker accepts may hold a staged warning, its
diagnostics must equal the staged part of Go's (empty but for XO0804).
selfhost/check/flow.xo ports flow.go and the mutability rules of stmt.go
and call.go: the call information of check.go (callee, receiver, and
`var` arguments) is a table keyed by the call's expression key, and
check.go identObj is the name records (uses) plus `decl_at` (the names a
pattern or `var` declares); identifiers, calls, and loops are keyed by
offset where Go keys by node pointer. internal/selfhost/testdata/flow
holds one module per rule (moves, lost and stored updates, mutability
and aliases, unused bindings, split sections), compared like every
other run. The mutations add value semantics edits (valueEdits: a `var`
removed from a binding, parameter, or receiver, an added binding never
read, a store back into a collection dropped, a builder frozen twice).

**Comparison rules of stage 6** (2026-10-10, patterns and
exhaustiveness; they extend stage 5's): the staged diagnostics add
XO0301 (a match that is not exhaustive, with up to five missing
patterns), XO0303 (a refutable `let` or `for` pattern), and the XO0305
warning (a `_` or catch all arm hiding variants of an enum of the
module). selfhost/check/exhaust.xo ports the usefulness algorithm of
pattern.go with its witnesses; pattern.xo returns the pattern forms
(`IPat`, pattern.go ipat) that the algorithm reads.
internal/selfhost/testdata/exhaust holds modules for enums (nested,
guarded, or patterns, uninhabited variants), Bool, tuples, lists with
`..`, structs, newtypes, numbers and strings, refutable `let` and `for`
patterns, and the XO0305 rule with its default arm threshold. The
mutations add pattern edits (patternEdits: a one line match arm
removed, or its pattern replaced by `_`). A run the Xo checker cannot
parse is now checked too: the Go parser must report a hard error in the
same file (`habitOnly` false), or the run fails.

**The Xo checker** keeps the graph in tables: objects, modules, and
files are entries of Lists in one `Checker` value and refer to each
other by index (`-1` for none); scopes are a List of `Map[Bytes, Int]`;
the name records are a List with a map from file and offset to its
entry. A type is an Int into the type table `tys`; an inference variable
is an entry whose `ref` is written in place when unification binds it,
and every binding is pushed on a trail so a failed tentative unification
is undone (infer.go's `mark` and `undo`). A substitution is a `Map[Int,
Int]` from type parameter objects to types (an effect parameter maps to
an effect argument type). Effect sets are Lists of items (labels,
effect parameters, effect variables) in a table of their own, with
effect variables bound like type variables. An expression is identified
by its file, span, and kind (the Go checker uses node pointers).

## Consequences

- A change to name resolution in internal/check must be made in
  selfhost/check in the same change, or `go test -short ./...` fails.
- The Go checker carries a record per resolved identifier; it never
  influences what the checker accepts.
- Stage 1 found three compiler bugs, fixed with goldens (nested
  optional `??` on the Go backend; same named types of two modules and
  the evaluation order of an assignment on the native backends,
  decision 0140).
- Stage 2 (2026-10-10) found a native backend bug (a `var` receiver read
  before the call's arguments, decision 0142) and a parser trap
  (`KIND_NAMES[k]` read as type arguments, decision 0143), both fixed
  with goldens, and an arm64 frame over the 28 KiB limit (gotcha
  arm64-large-frame, worked around). Every accepted repository run (348)
  has identical names and types sections on the three backends, and the
  76 runs rejected only for names and types get the same diagnostics.
- A change to type checking in internal/check (a rule, a diagnostic of
  the stage 2 list, a name record) must be made in selfhost/check in the
  same change, or `go test -short ./...` fails.
- Stages 3 and 4 (2026-10-10) found no compiler bug and matched on the
  first full run: all 349 accepted repository runs have identical names,
  types, and sigs sections (inferred effects included) on the three
  backends, and the 91 runs rejected only with stage 1 to 4 diagnostics
  get the same diagnostics. A change to signatures, generics,
  comparability, effects, or the real-time rules in internal/check must
  be made in selfhost/check in the same change. Effect ceilings
  (XO1309) remain to port, with the loading of dependencies.

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
