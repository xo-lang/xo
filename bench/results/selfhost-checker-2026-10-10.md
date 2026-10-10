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
