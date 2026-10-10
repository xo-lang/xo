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
