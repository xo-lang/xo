# Self hosted checker, stage 1 (names), 2026-10-10

Decision 0141. The Xo checker (`selfhost/check`, 2,491 lines of Xo with
its CLI, dump, and 123 lines of tests) against internal/check (Go),
compared through the names and diags sections of `xo typedump`.

## What stage 1 covers

Loading as internal/check does: the directories under the named ones
(testdata, dot, and vendor directories skipped below the top), xo.mod
module paths, user imports through xo.mod roots, std declaration files
(prelude and testing always, others on import, transitively), import
cycles, duplicate imports, local names of import paths (`-` to `_`,
`/vN` skipped, keywords rejected). Declarations: top level functions,
types (with their type parameters and effect flags), constants,
protected names, duplicates. Type expressions: type parameters,
implicit receiver parameters (`fn (b: Box[T])`, and the zero argument
form), module and prelude types, `mod.T`, type arguments (skipping
effect arguments the way resolveArgsFor does). Signatures: methods
attached to their receiver type (interface, foreign, duplicate, and
field conflicts), parameters, results, error types. Bodies: every
statement and expression form, with Go's scoping (a `let` value before
its pattern, `if let` and `while let` scopes, or pattern alternatives,
`match` arms, `scope`, `select`, closures, `ensures` with `result`,
`old` only in `ensures`, tests with `os` and `testing`, defaults,
constants, and struct invariants in an empty scope), Python's `x = v`
habit (XO0601 then a declared `var`), module members (`m.f`, `m.T`,
`m.C`, calls, type argument values), pattern paths (`mod.Enum.V`,
`Enum.V`, `mod.Newtype(p)`), and unused imports.

Deferred to later stages, with no line on either side: fields, methods,
statics, variants and every bare UpperCamel value or callee not declared
in the module (`Red`, `Some(x)`, `Int(x)`: a variant of the expected
enum wins), named arguments, effects, loop labels. The record of what
is deferred is the list in decision 0141.

## Agreement

`TestSelfhostCheckerNames` checks every directory of the repository
with .xo files (597, std/, runs/, output/, dot directories excluded),
each alone, with the Xo checker built by the go, arm64, and llvm
backends:

| Go checker result | runs | Xo checker matches |
|---|---|---|
| accepted: identical names section, no Xo diagnostic | 347 | 347 |
| rejected only with name diagnostics: the same diagnostics | 56 | 56 |
| rejected for other reasons (parse habits XO0177, unused locals, types, effects, ...) | 178 | not compared |
| `use go` / `use c` (the Xo checker binds neither) | 16 | not compared |

Coverage: 403 of 597 runs compared and identical (347 + 56); the
accepted runs hold 49,346 name lines (29,431 locals, 11,951 types, 5,448
functions, 1,523 modules, 622 type parameters, 371 constants). The 24
checker goldens (internal/check/testdata/golden) are among the runs: 2
accepted and identical, 22 rejected for other reasons.

`TestSelfhostCheckerNamesMutations` renames an identifier, removes a
`use`, or duplicates a declaration in the 1-directory std-only modules
the Go checker accepts: 1,000 mutations (150 under `-short`) give 44
accepted and 563 rejected only for names, all identical. Two longer
runs, seed 7 with 3,000 and seed 11 with 4,000, gave 272 accepted and
4,043 names-only rejections, all identical (after the fixes below). The
mutations found the one disagreement left after the repository runs:
three segment pattern paths (`task.Timed.TimedOut`) whose module part
the Go checker resolves without types.

## Speed

Apple M4 Pro, 20 repetitions, ms per run; the Go figure is the full Go
checker (types, effects, flow included), so it is an upper bound for its
name resolution, which cannot be timed alone.

| input | Go checker (all phases) | Xo, Go backend | Xo, arm64 | Xo, LLVM |
|---|---|---|---|---|
| selfhost (lexer, parser, check: 8,496 lines, 3 modules) | 36.4 | 47.6 | 29.6 | 22.8 |
| selfhost/parser (with the lexer) | 28.0 | 35.4 | 22.4 | 17.0 |
| bench/xo/v0.4/08-notes-service (5 modules) | 8.5 | 10.8 | 6.1 | 4.7 |
| 614 directories, each alone, once | 2,128 total | 2,797 | 1,394 | 1,031 |

Parsing is most of it. On the selfhost input the Go parser takes 15.7
ms (user files 14.0, std 1.7), the Xo parser 39.5 (Go backend), 18.3
(arm64), 13.2 (LLVM). So the Xo name resolution itself costs about 8 ms
(Go backend), 11 ms (arm64), and 10 ms (LLVM) per run, against 20.7 ms
for all of the Go checker's work after parsing. arm64 being slower than
the Go backend on the names part (map lookups and inserts of
`Map[Bytes, Int]` dominate) is the next thing to profile:
`xo build --backend=arm64 --profile` is not there yet, so the next
experiment is timing `Map[Bytes, Int]` get and put in isolation on both
backends with a benchmark program under bench/perf.

## Graph shaped data in Xo

The checker's data is a graph: scopes map names to objects, objects
belong to modules and files, files import modules, types own type
parameters and methods. Xo values have no identity, so the checker keeps
every node in a table of one `Checker` value (`objs: List[Obj]`, `mods:
List[Module]`, `files: List[SrcFile]`) and links them by index, `-1`
for none. This worked without workarounds in the compiler:

- Reads and in place updates go through the table: `c.objs[o].tparams =
  ...`, `c.mods[m].members[name] = o`, `c.files[f].imports[key] = imp`.
  Nested place assignment through List and Map works on every backend
  (after decision 0140).
- Scopes are a `List[Map[Bytes, Int]]` pushed and popped; saving the
  walk state around a body is one struct copy (`Saved`), cheap because
  values are persistent.
- Friction: a Map value cannot be changed in place except through
  `update` with a closure, so marking an import used is get, copy,
  change, put. `-1` sentinels read better than `Int?` in index heavy
  code (`?? -1` at every lookup otherwise), at the cost of type safety;
  a typed index (`type ObjId struct(Int)`, a newtype) is the next
  refinement to try. Positional sub-patterns bind fields in declaration
  order, so `LetStmt(pattern, ..)` would bind the span; every arm names
  its fields (`LetStmt(pattern: pattern, ..)`), which is verbose.
  `fs.stat(p)` returns `FileInfo? ! FsErr`, and flattening it to a
  `FileInfo?` takes `.ok() ?? None`.

## Language and compiler gaps

Found and fixed, each with a golden:

1. Go backend: `??` on a nested optional (`T?? ?? None`) generated Go
   that did not compile (`internal/gen/testdata/golden/nested_option_coalesce`).
2. LLVM and arm64: two modules' types of the same name were one type
   (selfhost/check's `Diag` met the lexer's); user types whose name
   another module declares are now qualified by module path
   (`same_type_names`).
3. LLVM and arm64: an assignment read the target list before computing
   the value and dropped the value's pushes (decision 0140,
   `assign_rhs_mutates`).

Recorded, not fixed:

- `T??` cannot be written as a type (the lexer reads `??`); write
  `Option[T?]`. Fixing it touches both parsers; next step: split `??`
  into two `?` in type context in internal/parser and selfhost/parser
  with a golden.
- `use go` and `use c` runs (16) are skipped: the Xo checker does not
  read Go bindings. The bound declaration files are cached in the
  repository (`.xo/gobind/...`); next step: load a cached binding's
  declaration file as a std like module (path `go:<pkg>`) and count the
  runs whose cache exists.
- Function symbols on the native backends are qualified by a module's
  local name; two modules with the same local name and function name
  would clash (gotcha native-same-type-names-two-modules).

## Next stage

Stage 2 (types and inference): port the type representation
(`Named`, `Tuple`, `Func`, inference variables as indices into a
substitution table), literals, expected types, Option and Result rules,
field, method, and variant resolution, and compare the types section
on the same runs.

# Stage 2 (types and inference), 2026-10-10

Decision 0141, stage 2. The Xo checker (`selfhost/check`, 8,694 lines of
Xo with its CLI, dump, and 165 lines of tests; 2,491 at stage 1) now
types every expression the way internal/check does and prints the types
section of `xo typedump`; the names section gained the resolutions stage
1 deferred.

## What stage 2 covers

A port of internal/check's typed walk (expr.go, call.go, stmt.go,
pattern.go, infer.go, types.go, the type parts of decl.go): the type
table with inference variables bound in place and undone from a trail,
unification, assignability with the implicit conversions of core 2.10
(Some wrapping, interface conversion, function subsumption), error
conversion at `?`, interface satisfaction (structural, derives, the
builtin Eq, Hash, Ord, Json, Display rules, comparability), literals and
their expected types, operators (Duration and Time arithmetic, `??`,
`expect(x) == y`), calls with named arguments and defaults (closures
last), generic instantiation at calls and constraint checks, fields,
methods, statics, variants and payloads, newtypes, struct literals and
`with`, closures with declared or expected types, the Option and Result
rules (`?`, `Ok`/`Err` in return position, XO0201 to XO0206), `if`,
`match`, `select`, `scope` and its task failure rule, loops and labels,
constants, defaults, contracts, tests, patterns against their types,
effect sets as far as types print them (`fn() uses net`,
`Cache[K, V, E, net.dial]`), and the habits that recover a type after
XO0601 (`len(x)`, `append`, `xs.length`, `string`, ...).

The names section now records the field, method, static, variant, or
parameter an identifier names (`field ...`, `fn ... Type.name`,
`variant ...`, `param ...`), and prelude types called by name (`Int(x)`),
which stage 1 left out.

Deferred: the sigs section, XO0402, implicit receiver parameters as
printed in signatures (stage 3); checking effects, XO0701, inference of
non pub effects, ceilings (stage 4); value semantics (stage 5);
exhaustiveness (stage 6); notes and fixes (stage 7); the habit notes
(stage 8).

## Agreement

`TestSelfhostChecker` (go, arm64, and LLVM builds):

| Go checker result | runs | Xo checker matches |
|---|---|---|
| accepted: identical names and types sections, no Xo diagnostic | 348 | 348 |
| rejected only with stage 1 and 2 diagnostics: the same diagnostics | 76 | 76 |
| rejected for other reasons (parse habits XO0177, unused bindings, XO0402, effects, real-time, xo.mod) | 158 | not compared |
| `use go` / `use c` | 16 | not compared |

The accepted runs hold 91,173 name lines (37,069 locals, 18,458
functions and methods, 13,894 fields, 13,664 types, 5,305 variants, 1,608
modules, 622 type parameters, 372 constants, 181 parameters) and 116,419
type lines (276 function types with `uses`). The 76 rejected runs hold
113 errors: 84 of names (XO0601, XO0603, XO0110 on imports) and 29 of
types (XO0201, XO0202, XO0203, XO0204, XO0205, XO0206, XO0401, XO0403,
XO0404, XO0407, XO0410, XO0801, XO0803).

`TestSelfhostCheckerMutations` now makes type mutations (a literal of
another type, a `?` removed, a field or method name replaced) for half
the mutations: 1,000 gave 96 accepted and 790 rejected only for names
and types, all identical; the matching rejections held 250 XO0401, 70
XO0406, 21 XO0403, 18 XO0302, 11 XO0407, 7 XO0404, 7 XO0201, and 820
name errors. Seeds 7, 11, and 13 with 3,000 each: 795 accepted and 7,137
rejected only for names and types, all identical.

The first version matched 344 of the 347 accepted runs before effect
sets were added; the three others differed only in effect arguments
(`Cache[..., net.dial]`).

## Speed

Apple M4 Pro, 20 repetitions, ms per run, best of two. The Go figure is
the full Go checker (effects, flow, exhaustiveness included).

| input | Go checker (all phases) | Xo, Go backend | Xo, arm64 | Xo, LLVM |
|---|---|---|---|---|
| selfhost (lexer, parser, check: 14,830 lines, 3 modules) | 69.5 | 141.4 | 116.2 | 86.2 |
| selfhost/parser (with the lexer) | 28.4 | 56.6 | 46.7 | 34.3 |
| bench/xo/v0.4/08-notes-service (5 modules) | 8.5 | 25.2 | 12.6 | 9.1 |
| 598 directories, each alone, once | 2,086 total | 3,517 | 2,503 | 1,865 |

Parsing the selfhost files takes 26.2 ms in Go, 65.3 (Go backend), 30.9
(arm64), and 21.6 (LLVM) in Xo, so checking after parsing costs the Xo
checker about 76, 85, and 65 ms against about 41 ms for all of the Go
checker's phases: the LLVM build is 1.6 times the Go checker on the
work after parsing and 1.2 times overall. Two changes during the stage
took 10% off the LLVM build: name records keyed by an Int instead of a
`file:offset` string, and comparability decided once per type without
type arguments (it was the largest function in the samples: `==` on an
`Option[Type]` walked the AST types each time). The profile (macOS `sample`, LLVM
build) is now spread over the walk (`infer`, `check`, `set_type`, `subst`)
and reference counting of AST values (`xo.dup.Expr`, `xo.drop.Expr`,
about 5%). arm64 is again slower than the Go backend on the checking
part; the next experiment is the one stage 1 proposed (timing
`Map[Int, Int]` get and put in isolation on both backends under
bench/perf) together with timing `c.tys[t].kind` reads of a 9 field
struct in a List, the checker's most frequent operation.

## Types in tables

Types are Ints into one `tys: List[Ty]` of the Checker; a `Ty` holds its
kind, object, argument list, and for variables the bound type. Binding a
variable is `c.tys[v].ref = t` plus a push on the trail, so undoing a
failed tentative unification is a loop over the trail. Effect variables
use the same trail (negative entries). This needed nothing new from the
language. Two things cost an attempt each:

- `KIND_NAMES[k]` (a constant list indexed by a local) parsed as type
  arguments; fixed in both parsers (decision 0143).
- On the native backends `c.implements(t, c.named("Display"))` lost the
  type `named` pushed: the `var` receiver was read before the arguments
  (decision 0142).

Value semantics shaped the walk in one place: Go's checker passes `*Var`
pointers around and binds them; a `Var` here is an index, so every type
that mentions it sees the binding without any sharing of values.

## Language and compiler gaps

Found and fixed, each with a golden:

1. Native backends: a `var` receiver or argument was read before the
   call's arguments, dropping their changes to it (decision 0142,
   `var_recv_after_args`).
2. Parsers: a constant indexed by a lowercase name was read as type
   arguments (decision 0143, `const_index`).

Recorded, not fixed:

- arm64: one spill slot per value made `habit_call` (a 150 line
  `match`) exceed the 28 KiB frame limit after a field was added to the
  Checker; split into one function per habit. Next step for the backend:
  reuse spill slots of values not live at the same time (gotcha
  arm64-large-frame).
- `T??` still cannot be written as a type (stage 1); not needed by stage
  2.
- `use go` and `use c` runs (16) remain skipped (stage 1's next step
  applies).

## Next stage

Stage 3 (generics): the sigs section (type parameters, implicit receiver
parameters, constraints), XO0402 for type arguments nothing determined,
and generic declarations' bodies compared through the types of their
expressions, which already print with type parameters.

# Stages 3 (generics) and 4 (effects and real-time), 2026-10-10

Decision 0141, stages 3 and 4. The Xo checker (`selfhost/check`, 9,749
lines of Xo with its CLI, dump, and 165 lines of tests; 8,694 at stage
2) now prints the sigs section of `xo typedump` and reports the
generics, effects, and real-time diagnostics.

## What stages 3 and 4 cover

Stage 3 (generics; dump.xo, comparable.xo, call.xo):

- The sigs section: every function, method, static, and interface
  method of a module, sorted by position, with its type parameters
  (implicit receiver parameters and elided effect parameters marked
  `implicit`, constraints, `: Effects`), receiver, parameters, result,
  error type, and declared or inferred effects (dump.go sigText).
- XO0402 for the type parameters of a call that nothing determined
  (check.go typeArgVars, reported after the bodies, once per callee
  span, after the closures whose body never completes get `Never`).
- XO0404 on comparability (decision 0112): `derive Eq` or `Hash` on a
  type that holds a handle or a function value (before the bodies), and
  Map keys and Set elements that are not comparable, at the type
  expression that writes them (decl.go resolveType) and at the first
  expression of a file whose type holds them (comparable.go
  checkKeyTypes), once per file and type. Constraint checks at calls
  were already part of stage 2.
- Generic bodies were already compared through the types section (their
  expressions print with type parameters); stage 3 adds nothing there.

Stage 4 (effects and real-time; effects.xo, realtime.xo):

- Effect items that stand for the inferred effects of a function that
  is not pub (a call, or the function used as a value), each with the
  substitution of its effect parameters at the call, so the effect
  arguments of the call (`fx` bound to a closure's effects) reach the
  caller.
- Inference of non pub effects: a fixpoint over the bodies of every
  function without `uses` that is not pub (effects.go solveEffects),
  printed in the sigs section.
- XO0701 obligations: function declarations with `uses` (or pub without
  it), closures with `uses`, function values passed where a function type
  is expected (subsumption), methods implementing an interface method,
  and contract clauses and struct invariants (pure). Assignability and
  interface satisfaction now take the span that records an obligation,
  as in Go (`assignable(src, dst, sp)`).
- Real-time functions (core 6.5, decision 0133): XO1401 to XO1405 by a
  walk of each `uses realtime` function after the bodies are checked,
  on the recorded expression types and calls (a call of a function value
  is recorded for XO1403), cycles among real-time functions (XO1404),
  effects a real-time function declares (XO1405), and loop bounds
  (`bound N`: constant, not negative, not below a constant range's
  length; XO1402).

Not ported: effect ceilings of dependencies (XO1309). No repository run
requires a dependency, so typedump has nothing to compare, and the Xo
checker does not load modules from the module cache. Next step: load
`require` lines and module cache directories in load.xo (as
internal/check/deps.go does), then port ceiling.go and compare on a
testdata workspace with a ceiling (internal/mod/testdata has repos to
start from). Notes (the call chain of XO0701, elided effect notes) are
stage 7.

## Agreement

`TestSelfhostChecker` (go, arm64, and LLVM builds) compares the names,
types, and sigs sections (effects included) and the diagnostics of
stages 1 to 4:

| Go checker result | runs | Xo checker matches |
|---|---|---|
| accepted: identical names, types, and sigs sections, no Xo diagnostic | 349 | 349 |
| rejected only with stage 1 to 4 diagnostics: the same diagnostics | 91 | 91 |
| rejected for other reasons (parse habits XO0177, unused bindings, value semantics, exhaustiveness, xo.mod, time layouts) | 143 | not compared; 14 still get exactly the staged part of Go's diagnostics |
| `use go` / `use c` | 16 | not compared |

Stage 3 alone (sigs without effects, stage 1 to 3 diagnostics) matched
349 accepted and 85 rejected runs on the three backends. With stage 4,
six more runs are compared (the realtime and effects goldens of
internal/check, `codex-elided-effect` of internal/fix). The 91 matching
rejections hold 17 XO0402, 7 XO0404, 2 XO0701, 13 XO1401, 8 XO1402, 5
XO1403, 2 XO1404, and 7 XO1405 next to the names and types diagnostics.
Every accepted run's sigs section matches, inferred effects included.

`TestSelfhostCheckerMutations` now draws a quarter each of name, type,
generics (a type argument removed, a constraint dropped), and effects (a
`uses` clause removed, or one added: an effect, or `realtime`, which
turns on the rules of core 6.5 for any function) mutations. 1,000
mutations (the default seed): 228 accepted and 728 rejected only with
staged diagnostics, all identical (before `realtime` was among the
additions). Seeds 7 and 11 with 3,000 each (with `realtime`): 1,151
accepted and 4,476 staged rejections, all identical; they held 803
XO0701, 2,611 XO1401, 181 XO1402, 2,424 XO1403, 18 XO1404, 1,774
XO1405, 201 XO0402, and 54 XO0404. Seed 13 with 3,000 on the LLVM and
arm64 builds (`XO_SELFHOST_BACKEND`): 601 accepted and 2,210 staged
rejections, identical on both.

Nothing new disagreed: the port matched on the first full run of each
stage; the work was in reading internal/check closely (two places check
Map keys; obligations are recorded inside assignability and interface
satisfaction, and survive a later failure of the same check; the
function items of a closure's set get the substitution of the call that
passes it).

## Speed

Apple M4 Pro, 20 repetitions (all directories: once), ms per run, best
of two. The Go figure is the full Go checker (flow, exhaustiveness,
habits included).

| input | Go checker (all phases) | Xo, Go backend | Xo, arm64 | Xo, LLVM | LLVM / Go |
|---|---|---|---|---|---|
| selfhost (lexer, parser, check: 15,930 lines, 3 modules) | 82.9 | 165.1 | 142.5 | 106.0 | 1.28 |
| selfhost/parser (with the lexer) | 28.7 | 62.2 | 53.3 | 39.6 | 1.38 |
| bench/xo/v0.4/08-notes-service (5 modules) | 8.5 | 27.5 | 14.2 | 10.2 | 1.20 |
| 599 directories, each alone, once | 2,097 total | 3,646 | 2,721 | 2,029 | 0.97 |

At stage 2 the LLVM build took 34.3 ms on selfhost/parser and 9.1 on the
notes service; stages 3 and 4 add about 5 and 1 ms. The new passes run
over every recorded expression type (Map and Set keys) and every body's
effects (the fixpoint), as Go's do. Next experiment: time
check_key_types and solve_effects alone in `--bench` mode (os.clock
around each pass, printed with the total) on the selfhost input, and
replace the `List.contains` sets of `Flat` with `Map[Str, Bool]` if the
fixpoint dominates.

## Language and compiler gaps

None found in these stages. The arm64 28 KiB frame limit (stage 2) did
not recur: the new functions are small, and `rt_expr`, the largest
`match`, compiles on arm64.

## Next stage

Stage 5 (value semantics, diags): mutation of immutable bindings and
`var` receivers (XO0501, XO0502 beyond assignment targets), aliasing of
`var` arguments (XO0503), moves (XO0510), and lost updates (XO0520),
ported from flow.go; with unused bindings (XO0110 and XO0201 on
bindings, in 15 of the 143 runs not compared). The parse habits
(XO0177, stage 8) are the only other errors of 99 of them (the bench
corpora of other languages' habits), so porting the parser's habit
diagnostics ahead of stage 8 would open the most runs to comparison:
next experiment, list the XO0177 messages of those 99 runs (`xo
typedump --section=diags`) and check which the Xo parser already
reports.

# Parse habits, stage 5 (value semantics), stage 6 (exhaustiveness), 2026-10-10

Decision 0141. The Xo parser (`selfhost/parser`, 7,173 lines with its
tests; habits*.xo are new) reports the habits of other languages and the
other soft errors of internal/parser and recovers the same tree; the Xo
checker (`selfhost/check`, 12,021 lines; flow.xo and exhaust.xo are new)
checks that tree and adds the value semantics rules and exhaustiveness.

## What the three steps cover

1. Parse habits, pulled ahead of stage 8 because they were the only
   errors of 99 of the 143 runs not compared (the 96 one-habit fixtures
   of internal/fix/testdata/habits, listed with `xo typedump
   --section=diags`: one XO0177 each, 86 distinct messages, plus
   the XO0150, XO0152, XO0153, XO0169, XO0173, XO0175, and XO0178
   fixtures). A port of habits.go, habits_lex.go, habits_decl.go,
   habits_stmt.go, habits_expr.go, habits_type.go, and leading_op.go
   with their messages, spans, and recovery (the Xo form each habit
   stands for), without the machine fixes and notes (stage 7). The soft
   errors of the Go parser (XO0161, XO0162 after `}`, XO0165 after `is`,
   XO0169, XO0170 in a parameter list, XO0173, XO0174, XO0175 between
   arms, XO0176, XO0178) no longer stop the Xo parser; lexer diagnostics
   are kept and the parse goes on, as in Go; a hard error still stops
   it. `class` is a hard error in Go that resynchronizes at the next
   declaration; the Xo parser does the same for it.
2. Stage 5, value semantics (flow.xo): `var` places (XO0501 on `var`
   receivers, XO0502 on assignments, `var` arguments, and captures),
   overlapping `var` arguments (XO0503, decision 0111), moves of move
   only values (XO0510), lost updates of copies out of collections and
   of values stored into them (XO0520), split critical sections (XO0804,
   a warning), and bindings never read (XO0110, and XO0201 for a Result).
3. Stage 6, patterns (exhaust.xo): the usefulness algorithm with
   witnesses, for matches that are not exhaustive (XO0301, up to five
   missing patterns), refutable `let` and `for` patterns (XO0303), and
   `_` arms hiding variants of an enum of the module (XO0305, a warning,
   with the default arm threshold of decision 0080).

## Agreement

`TestSelfhostParserDifferential` (go, arm64, llvm) now compares the
whole dump, diagnostics included, of every file the Xo parser parses to
the end, and requires that every file whose Go diagnostics are all
habits be parsed to the end: 107 of the 109 rejected repository files
(the other 2 have hard errors), 948 to 981 of 20,000 token mutations per
seed (seeds 1, 2, 3, and the default), and 9,605 of 10,000 inputs with
one to three habit edits of repository files (the new `habits` subtest;
the rest have no habit left after the edit or a hard error), all
identical.

`TestSelfhostChecker` (go, arm64, and LLVM builds):

| Go checker result | runs | Xo checker matches |
|---|---|---|
| accepted: identical names, types, and sigs sections, the same (staged) warnings | 350 | 350 |
| rejected only with diagnostics of stages 1 to 6 and the parser: the same diagnostics | 230 | 230 |
| rejected for other reasons | 12 | not compared; 3 get exactly the staged part |
| `use go` / `use c` | 16 | not compared |

Coverage: 580 of 608 runs compared and identical (440 of 599 at stage
4). The 230 matching rejections hold, besides the stage 1 to 4 codes,
101 XO0177, 4 XO0178, 3 XO0175, 2 XO0169, 1 XO0173, 2 XO0150, 1 XO0152,
1 XO0153 (parser), 86 XO0110, 6 XO0201, 7 XO0501, 12 XO0502, 4 XO0503,
10 XO0510, 11 XO0520, 1 XO0804 (stage 5), 21 XO0301, 5 XO0303, and 8
XO0305 (stage 6). internal/selfhost/testdata/flow (moves, lost, stored,
mutability, unused, sections) and testdata/exhaust (enums, shapes,
wildcard) are new modules written to exercise each rule.

Runs still not compared, and why:

- 2 bench/paper/v0.1 runs (02-worker-pool, 05-rate-limiter): hard parse
  errors (XO0160); the Go parser resynchronizes and reports more, the
  Xo parser stops. Porting the resynchronization (syncStmt, syncTop, the
  member and arm loops) is the next parser step; the test already checks
  that such a run has a hard Go parse error.
- 5 internal/check/testdata/golden/imports_deny runs (XO1301, XO1312)
  and internal/mod/testdata/.../invoice (XO1302): import deny lists and
  dependency rules of xo.mod, not ported (they need the module loading
  that effect ceilings, XO1309, need too).
- internal/check/testdata/golden/naming and fix/testdata/habits/
  go-exported-name (XO0101, naming conventions), and golden/time_layout
  and fix/testdata/habits/time-layout-two-digit-year (XO0420 and a
  checker XO0177): the checker's habits, stage 8.
- 16 `use go`/`use c` runs (stage 1's next step applies).

`TestSelfhostCheckerMutations` now draws a seventh each of name, type,
generics, effects, habit (`x := v`, `val`, `let mut`, `func`/`def`/
`function`, `and`/`or`, `x++`, `?:`, `...`, `elif`, `loop`, `throw`, `#`
comments, Go parameters, `): T`, `===`, `[]T`, `map[K]V`, `List<T>`),
value semantics (a `var` removed from a binding, parameter, or
receiver, a copy never read, a store back dropped, a builder frozen
twice), and pattern (a one line arm removed, or its pattern replaced by
`_`) mutations. Seeds 7, 11, and 13 with 3,000 each (go backend): 1,248
accepted and 7,605 rejected only with staged diagnostics, all identical;
seed 19 on LLVM and 23 on arm64 (3,000 each): 826 accepted and 5,089
staged rejections, identical. They held 418 to 479 XO0177, 400 to 500
XO0110, 50 to 80 XO0301, 80 to 100 XO0305, 70 to 120 XO0501, 140 to 170
XO0502, and 20 to 55 XO0804 per seed. The 41 to 55 rejected otherwise
per seed are mutations with a hard parse error (33 to 47) and naming
diagnostics (XO0101).

Nothing disagreed on the first full run of each step. The parse habits
took one fix after the first comparison (two trees differed: Go reads
`p.tok.Span` after `expectIdent` in one composite literal, and an
`impl` method left with no parameters keeps an empty non nil list;
gotchas go-composite-literal-reads-after-call and
go-empty-list-not-nil-in-tree), and the token mutations found one more
(diagnostic quoting, below).

## Speed

Apple M4 Pro, 20 repetitions (all directories: once), ms per run, best
of two. The Go figure is the full Go checker.

| input | Go checker | Xo, Go backend | Xo, arm64 | Xo, LLVM | LLVM / Go |
|---|---|---|---|---|---|
| selfhost (lexer, parser, check: 21,045 lines, 3 modules) | 111.9 | 281.8 | 278.5 | 210.2 | 1.88 |
| selfhost/parser (with the lexer) | 40.5 | 120.6 | 119.1 | 88.4 | 2.18 |
| bench/xo/v0.4/08-notes-service (5 modules) | 8.5 | 33.9 | 18.8 | 13.9 | 1.64 |
| 608 directories, each alone, once | 2,139 total | 5,151 | 3,810 | 2,857 | 1.34 |

The stage 4 build of the checker (01ac2fd) takes 143 and 61.5 ms on the
first two inputs today, so these steps add 47% (it was 1.28 times the Go
checker then): the habits make the Xo parser 35% slower (the selfhost
sources: 28.5 ms before, 38.5 ms now with the LLVM build, Go's parser
35.7 ms), the flow checks cost 9 ms on selfhost/parser and 21 ms on
selfhost (measured by leaving them out), exhaustiveness 2 and 8 ms. The
first version of the flow checks walked each body once per candidate
`var` with a list of child nodes built at every node (3.5 times the Go
checker); one walk for all candidates, direct recursion, and skipping
the move walk of bodies with no move only value took that to 1.9. The
parser's habit checks are guarded by the token kinds they can start with.
Next experiments: fold the inspect passes of the flow checks (mutating,
storing, and the kinds of stored updates; split sections) into the one
flow walk, ordered as walk.go's preorder; and in the parser, time
`lex_habits` alone (it copies every token, as the old `x++` pass did)
against a version that rewrites only around the habit tokens.

## Language and compiler gaps

Found and fixed: the Xo parser dump quoted diagnostic messages with the
token dump's quoting (`\x0a` for a newline) where parser.Dump uses
strconv.Quote; a habit message holds source text. `Bytes.quote()` is
strconv.Quote on every backend (gotchas/parser-dump-message-quoting.md).

Recorded: the arm64 28 KiB frame limit recurred a third time. A
`Map[Int, Bool]` field added to the `Checker` (the read flag of locals
beside the objects) pushed `resolve_named_type` to 29,072 bytes; the
flag went back into `Obj` (gotchas/arm64-large-frame.md). The backend's
next step is unchanged: reuse spill slots of values not live at the same
time.

No compiler bug was found in these steps; the value semantics and
pattern code is a direct port, and every backend agreed.

## Next stage

Stage 7 (diagnostics with notes and fixes, a full JSON section): the
notes and machine fixes of every diagnostic ported so far, the habits'
fixes first (habits*.go `addFix` and `addNote` are the parts left out of
the port). In parallel, the parser's resynchronization after a hard
error, which would let the two bench/paper runs and every rejected
token mutation be compared whole.

# Parser resynchronization, arm64 frames, stages 7 and 8, 2026-10-10

Decisions 0141, 0127, 0146.

## arm64 frames (decision 0146)

The 28 KiB frame limit that blocked the checker three times is gone.
Spill slots are shared by values whose live ranges do not overlap: the
largest frame of the checker went from 28,176 bytes
(`resolve_named_type`) to 6,448 (`check0`), the sum of its 809 frames
from 2.28 MB to 1.10 MB, and the arm64 build is about 4% faster on the
selfhost input (5,547 to 5,349 ms for 20 runs). A frame past 28 KiB
keeps x28 free and addresses its far slots through it
(`internal/arm64/testdata/run/huge_frame`, 36 KiB, runs on a fiber too).
The split `habit_*` functions and the `used` flag in `Obj` stay: both
read well and neither is needed any more.

## Parser resynchronization

The Xo parser now goes on after a hard error as internal/parser does
(`bad`, `sync_stmt`, `sync_top`) and prints the recovered tree;
`TestSelfhostParserDifferential` compares every file's dump, rejected
ones included. The first comparison found the error paths the port had
simplified while nothing after a hard error was compared (messages,
`while` conditions as expressions, `..` patterns, `derive` after a
failed body, struct literals in conditions;
gotchas/selfhost-parser-error-paths.md). After the fixes: the 879
repository files (109 rejected, 3 with hard errors), the spec blocks
(75 rejected, all with hard errors), and 20,000 token mutations per seed
(seeds 1, 2, 3, and the default: about 19,800 rejected per seed, 19,700
of them with hard errors) give byte identical dumps.

The checker then checks runs with hard parse errors like the Go checker
(it no longer marks them unsupported): the two bench/paper/v0.1 runs
(02-worker-pool, 05-rate-limiter) are compared and identical, and the
mutations with a hard parse error (21 XO0160, 11 XO0172, 7 XO0165 of
seed 7 with 3,000) match too. One Go checker bug was found on the way:
`xo typedump` panicked on a use of an unresolved import (a typed nil
`*Module` in a name record; gotchas/go-typed-nil-name-record.md,
`TestDumpUnresolvedImport`).

## Stage 7: notes and fixes, the full JSON section

`xo typedump --section=full` prints each diagnostic as `xo check --json`
does (severity, notes, fixes); the Xo checker prints the same section
and `TestSelfhostChecker` and `TestSelfhostCheckerMutations` compare it
on every compared run. `xo astdump` prints the notes and fixes of each
parser diagnostic, so the parser differential compares them on every
file and mutation.

Ported: every note and fix of internal/parser (parser.go, habits*.go,
leading_op.go), the checker's notes and fixes of the stage 1 to 6
diagnostics (edit distance suggestions with `nearest` and
`confidentGuess` for every XO0601 and named argument, previous
declarations, comparability, numeric conversions, Result and optional
fixes, missing match arms and struct fields as holes, `if` without
`else`, moves and lost updates, real-time notes and `bound N` fixes, the
XO0701 call chain, which needed the places where effects are performed:
effect items now carry a file, span, and callee), and the checker's
habits (habits.go). Selfhost/check/notes.xo is new.

Agreement (go, arm64, LLVM builds): the 351 accepted and 232 rejected
compared repository runs give identical full sections; 3,000 mutations
of seed 7 (2,588 rejected with staged diagnostics, 243 of them XO0701)
too; the parser differential (repository, spec blocks, 20,000 token
mutations of seeds 1 and the default) gives identical notes and fixes.

Found on the way, a Go bug: `diag.List.Errorf` returned a pointer into a
slice of values, so a note or fix attached after more diagnostics were
reported (a parsed closure body, a checked argument) was lost whenever
the slice had grown; `Items` holds pointers now
(gotchas/diag-pointer-after-append.md). One golden of internal/fix
(py-map-builtin) gained the fix it had lost. And json.Marshal's HTML
escaping inside `Diagnostic.MarshalJSON` (gotchas/json-marshaler-escapes-html.md).

## Stage 8: naming, time layouts, the checker's habits

Ported: check.go `checkName` at the 25 places internal/check calls it,
in the same order (selfhost/check/naming.xo: XO0101, the habit note of
Go's capitalized exports, camelCase, or a lowercase type, and the
rename fix, which `finish_renames` extends to every use of a local
after the other passes, as check.go `finishRenames` does), and
timelayout.go (selfhost/check/timelayout.xo: constant layouts of
`time.format`, `time.parse`, and `time.check_layout`, XO0420 with its
did you mean fix, and the XO0177 habit of Java, strftime, and Go
reference date patterns with the rewrite to named fields). XO0101,
XO0420, and the checker's XO0177 are staged diagnostics now.

Coverage (go, arm64, and LLVM builds): of 609 repository runs, 351 are
accepted and 236 rejected only with staged diagnostics, all 587 with
identical names, types, sigs, diags, and full sections. The 4 runs
stage 8 opened match: internal/check/testdata/golden/naming,
golden/time_layout, internal/fix/testdata/habits/go-exported-name, and
habits/time-layout-two-digit-year. The mutations gained an eighth kind,
naming (a declared name and every use the lexer sees respelled against
its convention): seed 8 with 3,000 mutations (263 with XO0101), seed 9
with 1,500 on the LLVM build, and seed 10 with 1,500 on arm64 give 0
differences.

Runs not compared, and why:

| runs | why |
|---|---|
| 16 (`examples/realtime-control`, 15 internal/gen goldens and replays) | `use go` or `use c`: the Xo loader has no Go or C interop and marks them unsupported |
| 5 (`internal/check/testdata/golden/imports_deny/*`) | import deny rules of xo.mod (XO1301, XO1312), not ported |
| 1 (`internal/mod/testdata/repos/github.com/acme/billing@v1.2.0/invoice`) | dependency rule XO1302, not ported |

Two of the 6 dependency rule runs give the same staged diagnostics; the
rest of their output is not compared.

Found on the way: a mutation that duplicates a real-time function
(`process` declared twice) made the Xo checker fault. The real-time
pass walks the duplicate's body, which is never type checked, and
`named_obj` indexed the type table with -1 (the Go checker's nil type
falls through). `named_obj` returns -1 for no type now.

Speed (Apple M4 Pro, 20 runs, best of two, ms per run; the machine
was slower today than for the stage 6 table: the Go checker's figure
went from 111.9 to 136.8 ms on selfhost, which also grew by 3,600
lines):

| input | Go checker | Xo, Go backend | Xo, arm64 | Xo, LLVM | LLVM / Go |
|---|---|---|---|---|---|
| selfhost (lexer, parser, check: 24,646 lines, 3 modules) | 136.8 | 339.1 | 349.5 | 270.7 | 1.98 |
| selfhost/parser (with the lexer) | 46.6 | 137.7 | 139.1 | 107.0 | 2.30 |
| bench/xo/v0.4/08-notes-service (5 modules) | 8.6 | 36.5 | 19.9 | 15.1 | 1.76 |

## The LLVM build's profile (decision 0145)

`sample` on the LLVM build checking selfhost 60 times (8 s, 5,900
samples on the main thread): bodies (`check_module`) 57%, loading and
parsing 20%, the flow checks 11%; by self time no function above 4%
(`set_type` 237 samples, `infer0` 233, `check0` 187), reference
counting about 10% (`xo_rl_free`, `xo.drop.*`, `xo.dup.*`). The
disassembly explained the spread: `Checker` (53 fields, about 1 KB) was
an aggregate argument and result of every `var` method, so each call
moved every field through the stack in the caller and again in the
callee's prologue and return (`set_type` was 9,689 instructions).

The fix, decision 0145: the LLVM backend passes a struct parameter of
256 bytes or more by reference, a `var` one written back through the
pointer, and a function forwards its own `var` parameter's memory to
the calls it makes. 20 runs of selfhost: 5,348 to 4,549 ms (15%), the
checker's output unchanged on every repository run and mutation.
Measured and not kept: loads and stores field by field (4,603 ms: the
unwind paths still keep every field loaded at entry live) and enums by
reference too (4,707 ms: `Expr` is 352 bytes, `Stmt` 1,336, but a
payload read stores the enum into an alloca first).

Next experiments, by expected gain:

1. Payload reads straight from a by reference enum (a `getelementptr`
   into the parameter's memory instead of the store to an alloca in
   `sfn.payload`), then enums by reference again: `check`, `infer`,
   and every `match` on `Expr` and `Stmt` pass 352 to 1,336 bytes by
   value today.
2. Lazy field loads of a by reference struct: lower `OpField` of a
   parameter to a load at the use, and the unwind exit's drops to
   loads from the reference, using the SSA's liveness to prove the
   memory still holds the value; this removes the copy of every field
   at entry that remains (`set_type` is still about 9,000 instructions,
   most of them the unwind path).
3. Reference counting: `xo_rl_free` and the `dup`/`drop` of `Expr` are
   the largest runtime entries; a borrow of match payloads would skip
   most of them.

# Module loading, 2026-10-10

Decisions 0141, 0086, 0095.

## What it covers

selfhost/check/modfile.xo (about 1,000 lines) ports what internal/mod
does when the checker loads a main module: xo.mod parsing with every
XO1301 problem (require lines, effect ceilings, `deny` rules, `replace`,
a missing `module` line), versions (semver order with prereleases, /vN
major paths), module paths, xo.work (found above the directory or named
by XO_WORK, ceilings on `use` lines), xo.lock (malformed lines, XO1304
mismatches, XO1305 missing hashes, the `/xo.mod` entries), minimal
version selection over the module cache (XO_MODCACHE, `!` escaped
paths), vendor/ (XO1307), and the content hashes of core 4.3 (SHA-256
of every file, nested modules skipped, base64 of the SHA-256 of the
listing: std/crypto). selfhost/check/deps.xo ports internal/check
deps.go, deny.go, and ceiling.go: XO1306 for imports no requirement
covers and for modules only in the build because another module
requires them, XO1312 with the rule's note, XO1301 for a rule naming no
package, XO1309 at calls and function values with the xo.mod or xo.work
line, and the dependency flag (no `uses realtime` note on a
dependency's function).

The Xo checker does not fetch. A version missing from the cache is
XO1303 under XO_OFFLINE; online it makes the run unsupported. The
differential tests pin both checkers to the fixture cache
internal/selfhost/testdata/modules/cache with XO_OFFLINE=1 and pass the
Go side's working directory as `--wd`, because the Go checker used to
`git clone` github.com/acme/money on every run of the billing fixture
(gotchas/checker-module-loading-depends-on-env.md).

## Fixtures

- internal/selfhost/testdata/modules/app: requires tick v1.0.0 `uses
  clock`, lib v1.0.0 `uses none` (which requires tick v1.1.0, so MVS
  selects v1.1.0, whose `now` adds `rand`), and example.com/Acme/util
  (cached as `example.com/!acme/util@v1.0.0`); two deny rules; a
  correct xo.lock. Runs: core (XO1312 and XO1309), web (XO1309 at a
  call and at a function value), ok (accepted), web/api (accepted). The
  cache modules are runs of their own (lib: XO1305 without a lock).
- internal/check/testdata/golden/effect_ceiling_work (new Go golden):
  ceilings on xo.work `use` lines, callbacks within `uses none`, a
  function value, two labels over.
- internal/check/testdata/golden/effect_ceiling_vendor (new Go golden):
  vendored dependencies with ceilings on their requires, and XO1306 for
  a module vendored only because another dependency requires it.

## Agreement

| backend | runs | accepted (match) | rejected only with staged diagnostics (match) | unsupported |
|---|---|---|---|---|
| go | 626 | 361 (361) | 249 (249) | 16 |
| arm64 | 626 | 361 (361) | 249 (249) | 16 |
| LLVM | 626 | 361 (361) | 249 (249) | 16 |

The six runs stage 8 left (imports_deny/* and acme/billing invoice)
matched on the first full run, as did the new fixtures. The 16 runs not
compared are the `use go` and `use c` ones (examples/realtime-control
and 15 internal/gen goldens and replays); no run is rejected for a
diagnostic the port lacks.

Module mutations (`TestSelfhostCheckerModuleMutations`, each a copy of
one fixture tree with one edit, every package directory a run):

| seed | backend | mutations | runs | accepted | rejected, staged | differences |
|---|---|---|---|---|---|---|
| 1 | go | 600 | 2,528 | 592 | 1,936 | 0 |
| 2 | go | 600 | 2,539 | 565 | 1,974 | 0 |
| 3 | go | 600 | 2,520 | 559 | 1,961 | 0 |
| 4 | LLVM | 400 | 1,693 | 330 | 1,363 | 0 |
| 5 | arm64 | 400 | 1,666 | 358 | 1,308 | 0 |

Seed 1's rejected runs hold 1,263 XO1309, 1,192 XO1312, 2,908 XO1301,
258 XO1306, 191 XO1303, 142 XO1305, 28 XO1304, 14 XO1307. No checker
or compiler bug was found; the one surprise was `requires` as a field
name (a contract keyword, gotchas/requires-is-a-keyword.md).

## Next

The checker is complete for programs without Go or C interop. The
remaining unsupported runs need `use go` (binding Go packages: the
gobind cache files are plain .xo declaration files, so loading an
existing binding is a small port; binding a new package runs `go list`)
and `use c` (headers). Next experiment for interop: load the cached
binding of `use go` imports when it exists (internal/check
loadGoImport with gobind.LoadOrBind's cache path) and report the run
unsupported only when a binding would have to be made.

# LLVM build experiments after module loading, 2026-10-10

Decisions 0145, 0147. The "Next experiments" of the LLVM profile above,
in order. Every measurement is `--bench=20` on the selfhost input (the
lexer, parser, and checker: 3 modules), interleaved round by round with
the build it is compared to (`ab.sh`: base, candidate, base, ...), ms
for 20 runs. Each LLVM build of the checker takes about 6.5 minutes of
clang -O2 on 84 MB of IR.

Baseline after module loading: LLVM 5,412 to 5,565 ms, the Go checker
(`xo typedump --bench=20 selfhost`) 2,939 to 2,968 ms: 1.88x. (The
stage 8 table's 4,549 ms after decision 0145 was measured on a quieter
machine; the checker has also grown by module loading.)

A profile of the baseline (`sample`, 8 s, 6,000 samples on the main
thread) has no function above 4.4% of self time: `infer0` 263, `set_type`
194, `pre_expr` 162, `xo_rl_free` 158, `parse_postfix` 148, `check0`
141, `xo.drop.Expr` 127, `xo.dup.Expr` 114, `memmove` 93. `set_type`, a
10 line function, compiles to 12,474 instructions; `infer0` to 74,000.

## 1. Enums by reference, payload reads from the reference (decision 0147)

| build | rounds (ms) |
|---|---|
| base | 5,565, 5,473, 5,451, 5,482 |
| enums by reference, tag and payload reads from the parameter's memory | 5,545, 5,486, 5,486, 5,496 |

Neutral: the 2 to 3% loss decision 0145 measured is gone, but nothing
is gained. The IR showed why: a sub expression passed to a call was
still loaded out of its box (44 words) and stored into the call's slot.

Then values in stable memory passed by address (decision 0147: a non
`var` by reference parameter's memory, its fields and payloads, and box
contents and list elements under a borrowed root):

| build | rounds (ms) |
|---|---|
| base | 5,473, 5,412, 5,419 |
| enums by reference and stable addresses | 5,181, 5,164, 5,156 |

5% faster; kept. The checker's output is identical on all 627
repository runs on the three backends.

## 2. Lazy field loads: `var` parameters stay in memory (decision 0148)

A dataflow over each function finds which SSA value the memory of a
`var` parameter passed by reference holds before each instruction
(calls store into it and `var` callees write back into it; `setfield`
of the held value is done in place; block parameters every edge passes
in place are held), so field reads load from memory, updates store one
field, calls and returns skip the store of what the memory already
holds, and values held at every use are never loaded whole.

| build | rounds (ms) |
|---|---|
| decision 0147 | 5,184, 5,115, 5,117 |
| values kept in memory | 3,114, 3,076, 3,081 |

40% faster; kept. The Go checker takes 2,939 to 2,968 ms on the same
input, so the LLVM build is now 1.05x the Go checker (from 1.88x at the
start of these experiments). The checker's output is identical on the
627 repository runs (three backends), 1,500 checker mutations (seed 11)
and 400 module mutations (seed 12) on the LLVM build. One bug on the
way (gotchas/llvm-held-memory-read-before-store.md): a return wrote one
`var` parameter back over the memory another held value was read from.

## 3. Borrowing match payloads (decision 0149)

After decisions 0147 and 0148, a profile (8 s, 5,900 samples) put
reference counting first: `xo_rl_free` 275, `xo.drop.Expr` 261,
`xo_alloc` 224, `xo.dup.Expr` 179, `memmove` 154, the `Stmt` drops and
dups about 160, `xo.drop.Token` 55. The node parameters of the checker's
recursive walk (`check`, `check0`, `infer` over `Expr`) were owned, as
some path consumes them, so each level duplicated the sub node it
projected and dropped its own.

The change: a parameter stays borrowed unless every normal path (not
landing pads) consumes it; the consuming paths retain it.

| build | rounds (ms) |
|---|---|
| decision 0148 | 3,148, 3,262, 3,542 |
| borrowed unless consumed on every path, all types | 76,516, 77,026, 76,601 |

25 times slower: `xo.map.put` became borrowed (one path does not
consume the map), so every insert retained the map and the in place
updates copied its lists (gotchas/borrowed-list-copied-on-update.md).
Restricted to enum parameters, which are never updated in place:

| build | rounds (ms) |
|---|---|
| decision 0148 | 3,203, 3,142, 3,144 |
| enum parameters borrowed unless consumed on every path | 2,962, 2,900, 2,939 |

7% faster; kept. Output identical on the 627 repository runs (three
backends; the RC change reaches arm64 too).

## Where the three experiments leave the checker

20 runs, best of two rounds, ms per run (Apple M4 Pro; same session for
every column):

| input | Go checker | Xo, Go backend | Xo, arm64 | Xo, LLVM | LLVM / Go |
|---|---|---|---|---|---|
| selfhost (lexer, parser, check: 3 modules) | 147.4 | 386.5 | 419.9 | 151.4 | 1.03 |
| selfhost/parser (with the lexer) | 47.6 | 146.9 | 159.4 | 55.2 | 1.16 |
| bench/xo/v0.4/08-notes-service (5 modules) | 9.0 | 39.2 | 25.1 | 9.1 | 1.01 |

The LLVM build went from 1.98x (stage 8) and 1.88x (after module
loading) to about parity with the Go checker. The LLVM binary shrank
from 29 MB to 16 MB; `infer0` from 297 KB of code to 50 KB, `set_type`
from 12,474 instructions to about 7,100.

Next experiments, by expected gain:

1. Port decisions 0145, 0147, and 0148 to the arm64 backend, which kept
   the field by field convention and is now 2.8x the Go checker on
   selfhost (the borrowing of 0149 reached it already, through the
   SSA). Start with `internal/arm64` call lowering of struct parameters
   of 256 bytes or more.
2. Drop the write back of zero values at the unwind exit of a function
   with a `var` parameter passed by reference: after a fault nobody
   reads that memory (the caller's pad holds nothing known there,
   held.go), and the zero `Checker` it builds (40 `rt.rl_empty` calls)
   is most of what is left of `set_type`'s 7,100 instructions. Measure
   code size and time.
3. Reference counting that remains: `xo_rl_free`, `xo_alloc`, and the
   parser's `xo.drop.Token` lead the profile; try the 0149 rule for
   structs whose leaves are all enums or boxes (no list or string leaf,
   so nothing is updated in place).
4. Track the home allocas too (held.go tracks only `var` parameters):
   a function without a `var` parameter of a type still stores each
   argument of that type into its alloca before every call.
