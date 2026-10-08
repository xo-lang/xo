# Toolchain and layout

Last verified: 2026-10-08

## Host

- Development host: macOS (darwin/arm64).
- Go: 1.26.1 (`go version`). The P1 prototype is written in Go and emits Go.
- Git repository, remote `git@github.com:josephspurrier/xo.git`, branch `main`
  (initialized 2026-10-08). `.gitignore` excludes `.tools/`, build outputs
  (`benchtask`, `*.exe`, `*.test`, `bin/`, `dist/`), logs, and editor files.
- `go test ./...` needs a C toolchain for the race detector tests (skipped
  when `go env CGO_ENABLED` is 0).

## Go module

- `go.mod` at the repo root: `module github.com/xo-lang/xo`, `go 1.26`.
  The module path intentionally stays `github.com/xo-lang/xo` even though the
  git remote is `github.com/josephspurrier/xo` (owner's choice, 2026-10-08).
  So `go install github.com/josephspurrier/xo/cmd/xo` does not work; build
  from a checkout with `go build -o .tools/bin/xo ./cmd/xo`.
- Standard library only. No third party dependencies.

## Commands

Run from the repo root.

```
go build ./...                           # build the compiler packages and std
go test ./...                            # all tests (lexer, parser, checker, spec examples, corpus, CLI, golden programs, race, cross compile)
go vet ./...
gofmt -l internal cmd std                # must print nothing
.tools/bin/staticcheck ./...             # must print nothing
go run ./cmd/xo parse [--json] [--ast] [--sig] <files or dirs...>
go run ./cmd/xo check [--json] <dirs...>
go run ./cmd/xo build [--targets=all|os/arch,...] [-o out] [--json] [--mono] [dir...]
go run ./cmd/xo run [--holes] [--mono] [dir] [-- args...]
go run ./cmd/xo test [--seed=N] [--run=text] [--race] [--real] [--cases=N] [--json] [-v] [--holes] [--mono] [dir...]
go test ./internal/gen -run TestGolden -golden-update   # rewrite want.txt of the golden programs
go test ./internal/gen -run TestStdTypes -update        # regenerate std/rt/std_gen.go from std/*.xo
go test -short ./...                                    # skips the race, cross compile, and CLI build tests
go test ./std/rt -run '^$' -bench . -benchmem           # runtime micro benchmarks (Map put/get/update/iter/copy/fork, Set, Eq, Hash)
bench/perf/run.sh                                       # Xo vs Go collection and generic workloads (see Benchmark tooling)
MONO=1 bench/perf/run.sh                                # adds an `xo build --mono` column
go test -run '^$' -fuzz=FuzzParse -fuzztime=60s ./internal/parser/   # optional fuzzing
go test ./internal/check -run TestGolden -update          # rewrite golden diagnostics after an intended change
go test ./internal/check -run TestCorpusV03 -corpus-report # rewrite the generated counts in bench/results/check-corpus-v0.4-spec.md
go test ./internal/check -run TestCorpusMutations -mutations -v   # slow: plant mutations in the corpus, count detections
```

`./...` is safe since `bench/go.mod` made `bench/` its own module (see
`gotchas/root-module-absorbs-bench-go.md`). Last verified: 2026-10-07.

`xo parse` walks directories for `.xo` files, prints diagnostics in a
`file:line:col: error XOnnnn: message` form on stderr (plus a summary line),
and exits 1 if any error was reported, 2 on usage errors. `--json` prints
`{"files": [...], "diagnostics": [...]}` on stdout with diagnostics in the core
10.2 shape (notes and fixes are always arrays). `--ast` adds each file's syntax
tree (every node has `kind` and `span` "line:col-line:col"). `--sig` accepts
function declarations without bodies (spec/std.md style signature listings).

`xo check` treats every directory under the given directories that holds
`.xo` files as a module (`testdata` and dot directories are skipped), loads
the user modules they import through the nearest `xo.mod` (`module <path>`:
`use <path>/<dir>` maps to `<xo.mod dir>/<dir>`), and loads std modules from
the embedded declaration files in `std/`. It prints `file:line:col: severity
XOnnnn: message` lines (with notes and fix titles) and a summary on stderr, or
with `--json` `{"modules": [...], "diagnostics": [...]}` on stdout. Exit status
is 1 when any error is reported (warnings such as XO0900 holes do not count),
2 on usage errors.

Diagnostic positions: lines and columns are 1 based, columns count bytes, and
spans are half open (`end_col` is one past the last character), which matches
the insertion fix example in core 10.2.

`xo build` checks (errors and holes block the build), generates Go, and runs
`go build -trimpath` with `CGO_ENABLED=0` per target; it prints one
`os/arch: path` line per binary. `--targets=all` is linux/amd64,
linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64, and
`wasi/wasm32` (Go's wasip1/wasm). Binaries go to `-o` (a file for one
target, a directory for several) or the build directory's `bin/`. `xo run`
builds for the host and runs with the arguments after the directory or
`--`, passing the exit status through. `xo test` builds the test blocks of
every root module into one test binary and runs it: it prints failures (and
passes with `-v`) and a summary line `ok: N passed, M failed, K skipped`;
exit status 1 when a test fails. Tests that declare `uses` run only with
`--real`, with real capabilities for the declared effects. Each test runs in
its module's directory (as `go test` does per package), in a fresh test
world (virtual clock, MemFs, FakeNet, ...); a test whose tasks are all
blocked with no timer pending fails as a deadlock; a test that runs over
60s of real time fails. Property tests (`test "x" for v: T`) and contract
fuzzing (every function with `requires`/`ensures` in a root module, core
9.1) run 100 generated cases (`--cases=N`) and shrink a failing input.
Last verified: 2026-10-07.

Build directories live in `$XO_BUILD_DIR` or `<user cache>/xo/build/<hash of
the module paths and mode>/` (stable, so Go's build cache is reused): `go.mod`
(module `xoprog`), `rt/` (the runtime, copied from the embedded `std/rt`),
`m/<module ident>/xo.go` per Xo module, and `main.go`. Crash bundles go to
`$XO_CRASH_DIR` or the system temporary directory. `XO_GO` overrides the go
command (default: `go` on PATH).

## Benchmark tooling

Last verified: 2026-10-07

- `bench/` is its own Go module (`bench/go.mod`, module `xobench`) so the
  root module never builds benchmark code. Each Go solution under
  `bench/go/<arm>/<task>/` also has its own `go.mod`.
- Go analyzers for the strict arm are installed project locally in
  `.tools/bin` (staticcheck, errcheck, nilaway, exhaustive), installed with
  `GOBIN=$PWD/.tools/bin go install <pkg>@latest`. The built `xo` binary is
  also placed there (`go build -o .tools/bin/xo ./cmd/xo`).
- `bench/go/strict-check.sh <dir>`: vet, staticcheck, errcheck, nilaway,
  exhaustive, `go test -race`.
- `bench/perf/run.sh`: builds `bench/perf/xo` (Xo) and `bench/perf/go`
  (hand written Go, its own `go.mod`), runs each workload `REPS` times
  (default 5), and prints a Markdown table of the best times and the
  ratio; checksums must agree. Workloads: collection (`putget get push
  update iter copymut branch`) and generic (`sortint sortpt hof maxby
  iface`, idiomatic Go generics on the Go side). `MONO=1` also builds with
  `xo build --mono` and adds a column, `XOFLAGS` passes flags to the
  default Xo build, `XO=<path>` uses another xo binary (for before and
  after runs), `WORKLOADS="..."` selects workloads, `PERF_OUT=<dir>` keeps
  the binaries. Last verified: 2026-10-08.
- `bench/planted/run_detection.py [task...]`: runs planted bug variants
  through the plain and strict loops, writes `bench/planted/results.jsonl`.

## Package layout (P1, implemented parts)

```
cmd/xo/                 CLI entry point; `xo parse`, `xo check`, `xo build`, `xo run`, `xo test`
internal/token/         token kinds, keywords, Pos and Span (file, line, col, offset)
internal/lexer/         tokens, newline termination (core 1.3), string interpolation parts,
                        doc comments
internal/ast/           syntax tree for the whole 0.4 grammar; JSON dump (ast.ToJSON/Dump)
internal/parser/        recursive descent parser with error recovery; SignatureMode
internal/diag/          diagnostics (core 10.2 JSON), codes.go is the registry of all codes
internal/check/         name resolution and type checking (see below)
internal/gen/           Go code generation with //line directives (see "Code generation")
internal/build/         check, generate, write the build directory, `go build` per target
std/                    std declaration files (prelude.xo, http.xo, json.xo, log.xo,
                        task.xo, testing.xo, time.xo) embedded by std/embed.go
std/rt/                 package rt, the runtime linked into every program (Go, embedded
                        by std/embed.go as std.Runtime and copied into each build)
```

`std/*.xo` are bodiless signature listings of `spec/std.md` (draft 0.4),
parsed in `parser.SignatureMode`. The encoding of std.md pseudo syntax
(`[var]` receivers, `Int.parse` statics, implicit receiver type parameters,
opaque handles, `@moves`) is documented at the top of `std/prelude.xo`. The
prelude imports `std/task` for `task.Group` (`os.tasks`, `s.group()`), so
`std/task` is always loaded. Last verified: 2026-10-07.

`internal/check` (package `check`, entry point `check.Check(dirs)`):

- `load.go`: module discovery, xo.mod roots, import resolution (XO0602,
  XO0604), std loading.
- `decl.go`: top level objects, type declarations, signatures, methods,
  effect clauses, type expressions (XO0101, XO0603, XO0606, XO0702).
- `types.go`, `infer.go`: types, substitution, unification with a trail,
  assignability with the three implicit conversions (core 2.10), interface
  satisfaction (structural, plus builtin rules for Eq, Hash, Ord, Json,
  Display, Debug), error conversion at `?`.
- `expr.go`, `call.go`, `stmt.go`: bidirectional checking of bodies. The
  expected type of a value is a type, a `retWant` (return position of
  `-> T ! E`), or `discard` (a value whose Result must be handled, XO0201).
- `pattern.go`: pattern typing and exhaustiveness (usefulness algorithm with
  witnesses, XO0301 to XO0304).
- `effects.go`: effect sets with labels, effect parameters, effect inference
  variables, and references to the inferred effects of non pub functions;
  fixpoint and XO0701 obligations with call chains.
- `flow.go`: moves of `Buf`/`StrBuilder` (XO0510) and lost updates (XO0520:
  copies out of a collection, including `.or_err(...)?`, and locals mutated
  after being stored into a collection), run after typing.
- `groups.go`: task groups (core 7.1): failure of group tasks (XO0801) and
  values holding a group escaping their scope (XO0802).
- Effect argument elision (core 6.2) is in `decl.go` (`resolveArgsFor`,
  `elidedEffect`): implicit effect variables in parameter and receiver
  types, implicit effect parameters on struct and enum fields, inference in
  local bindings and in result types.

Checker tests in `internal/check`: `rules_test.go` (table driven positive and
negative cases per rule), `planted_test.go` (the Xo side of
`bench/planted/`), `golden_test.go` with `testdata/golden/<case>/want.json`,
`rules04_test.go` (rules new in spec 0.4: XO0206, `is`, tuple fields, local
`const`, widened XO0520, task groups, effect elision, std 0.4 surface),
`corpus_test.go` (bench/paper/v0.3 as data under the current spec, and spec
Appendix A), `mutation_test.go` (opt in), `coverage_test.go` (every emitted
code has a positive case).

Parser notes for spec 0.4 (last verified 2026-10-07): `x is Pattern` is an
`ast.IsExpr` parsed at the precedence of `==` when the identifier `is`
follows an operand; a tuple field `t.0` is an `ast.SelectorExpr` whose `Sel`
is the decimal index (the lexer does not read a fraction after `.`); a local
`const` is an `ast.ConstDecl` statement; `&= |= ^= <<= >>=` are rejected; a
result type may start on the line after the parameters (`->` continuation,
std.md 7).

Parser tests live in `internal/parser`:

- `parser_test.go`: table driven expression, statement, pattern, declaration,
  doc comment, span, error recovery, and fix tests; a no panic test on
  truncated input; `FuzzParse`.
- `spec_test.go`: extracts every fenced block from `spec/core.md` and parses it
  as a file, then as a function body, then in signature mode. Non Xo blocks are
  in an explicit skip table, the 1.6 literal listing is lexed only, and spec
  bugs are listed in `specKnownFailures` (empty since spec 0.4).
  `spec/std.md` blocks are parsed in signature mode after rewriting `[var]`
  to `var`; pseudo signatures such as `Int.parse(s: Str) -> Int?` are dropped
  and logged.
- `corpus_test.go`: every `.xo` file under `bench/paper/v0.2/` must parse;
  `bench/paper/v0.1/` is parsed for information only.
- `sexpr_test.go`: compact S-expression printer used by the table tests.

## Build pipeline (P1)

`.xo` sources -> lexer -> parser -> checker -> Go source (internal/gen, with
`//line` directives) -> build directory with the runtime -> `go build` with
`GOOS`/`GOARCH` per target and `CGO_ENABLED=0` -> static binaries. All stages
are implemented. Last verified: 2026-10-07.

## Code generation

Last verified: 2026-10-07. Rules and choices; details in
`bench/results/gen-run-v0.4.md`.

- One Go package per Xo module (`xoprog/m/x_<module path>`), one file
  `xo.go`. Std modules map onto package `rt`. Go names: types keep their
  Xo name (std types are `<Module>_<Name>` in rt, `Http_Request`), payload
  types `<Enum>_<Variant>`, functions `F_<name>`, constants `C_<NAME>`,
  methods `M_<name>`, fields `F_<name>`, locals `v_<name>` (numbered when
  reused), temporaries `x_<kind><n>`, type parameters `tp_<Name>`.
- Every function, method, and closure takes `tk *rt.Task` first.
- Types: Int is int64 (sized types map to Go sized types), Float float64,
  Str string, Bytes `rt.Bytes` (a string), Rune `rt.Rune`, Duration and
  Time `rt.Duration`/`rt.Time` (int64 nanoseconds), `()` `rt.Unit`, tuples
  `rt.TupleN`, `T?` `rt.Option[T]` (a value: Some flag and V), `Result`
  `rt.Result[T, E]` (with the error frame trace), structs Go structs,
  enums `struct{ Tag int; P any }` with a payload struct per variant (so
  recursive enums work), interfaces Go interfaces with `M_` methods,
  `Display`/`Error`/`Debug` `rt.IDisplay`/`rt.IError`/`rt.IDebug`,
  `Chan`/`Mutex`/`Atomic` pointers to rt handles, List/Map/Set rt
  persistent collections.
- A function value of type `fn(A) -> R ! E` is `func(*rt.Task, A)
  rt.Result[R, E]`; one that cannot fail uses E = `rt.Never`, so the form
  never depends on whether a generic error parameter was instantiated with
  Never. Declared functions return R, `rt.Result[R, E]`, or nothing for
  `()`; a call whose declared error became Never takes `.V`. Declared
  functions used as values are wrapped in closures; subsumption (a function
  that cannot fail where a failing one is expected, a converting error)
  is an adapter closure.
- Generics are Go generics (constraints are `any`; Eq, Hash, Ord, Display
  go through rt functions that switch on the dynamic type or call the
  generated `XoEq`/`XoHash`/`XoCmp`/`XoDebug`/`M_display` methods). A
  method is a Go method unless it has its own type parameters, its
  receiver is a scalar prelude type, its receiver restricts the type's
  arguments (`List[Str].join`), or its signature instantiates its own
  receiver type with other arguments (Go instantiation cycle); then it is a
  top level function `<Type>_M_<name>` (user) or `<Type>_<name>` (rt).
- Expressions that need statements (blocks, if, match, `?`, `??`, scope,
  select, `with`) are hoisted; operands before one that needs statements
  are spilled into temporaries to keep left to right evaluation, except
  places (`var` receivers, `&` arguments). Blocks deliver their value to a
  destination: discard, assign, or return (so arms of a tail if/match
  return directly, core 2.6).
- A block that contains `defer` runs as `func() int { ... }()`; Go's
  defer gives block scoping, `tk.Shielded` the cancellation shield (core
  3.5, 7.2). `return`, `break`, `continue`, and `?` cross it as control
  codes dispatched after the call. `scope` is the same with
  `rt.Scope.Close` deferred.
- Integer arithmetic, conversions, indexing, and slicing call rt functions
  that fault with the values and the `file:line:col` of the operation;
  rt faults without a position use the innermost `.xo` frame of the stack.
- A struct field whose type contains the struct by value (through Option,
  Result, tuples, other structs) is stored in `rt.Box[T]`.
- Assignment resolves the place first (index operands left to right), then
  writes; through a box or a collection element it reads, modifies, and
  writes back (`m[k].count += 1`, `xs[i].push(x)` on a var receiver).
- `--mono` (`gen.Options.Mono`, `build.Options.Mono`, flag on `xo build`,
  `run`, `test`; default off; added 2026-10-08, internal/gen/mono.go):
  calls of user generic functions and methods with concrete type
  arguments call specialized copies `<name>__<types>` generated into the
  calling package (methods of generic types as top level copies, receiver
  first); `==`/`<` on a type parameter in a copy lower as for the concrete
  type, with typed `XoEqT`/`XoCmpT` methods generated for non generic
  structs and newtypes; constraint method calls call the concrete method;
  `List.map/filter/fold/any/all/find` with an inlinable closure literal
  become loops with the body inlined; a non capturing literal passed to a
  user generic function gets a `_lit` copy that inlines it; Int
  arithmetic uses `rt.AddI..RemI` and List indexing `AtM`. Fallback to the
  Go generic function: non concrete type arguments, copy chains deeper
  than 8, more than 64 copies of one function per package. Go forms are
  decided on the generic types, so copies keep the generic signature
  form. The build directory hash includes the mode. `TestGoldenMono` runs
  every golden program both ways. Numbers:
  `bench/results/perf-mono-2026-10-08.md`.
- The std structs, enums, and interfaces are generated into
  `std/rt/std_gen.go` by `gen.StdTypes`; `TestStdTypes` fails when it is
  stale. Opaque std handles and every std function are hand written in rt.

## Runtime (std/rt)

Last verified: 2026-10-07.

- `core.go` Option/Result/frames/tuples/Box; `arith.go` checked integer
  math; `fault.go` faults and contracts; `list.go`, `map.go` value
  semantics collections: List (shared backing array, append in place when
  the list ends at the claimed length) and Map/Set (append only entry log
  plus an atomic open addressing index; a write appends in place when the
  value's log length equals the claimed length, otherwise it forks a child
  table in O(1); soundness argument in the comment on `Map`, see
  `gotchas/map-copy-claims-log.md`; since 2026-10-08); `show.go` Eq/Hash/Cmp/Debug/Display; `str.go`, `time.go`,
  `option.go` prelude methods; `mono.go` typed helpers emitted only under
  `--mono` (`AddI`, `SubI`, `MulI`, `DivI`, `RemI` on int64, `CmpO`,
  `List.AtM`; same results and fault messages as the generic forms,
  checked by `mono_test.go`).
- `task.go`, `scope.go`, `chan.go`, `sync.go`: the cancellation tree,
  scopes, tasks, channels and `select` with explicit wait queues, Mutex,
  Atomic, Semaphore, `task.*`. `world.go`: the virtual clock of tests (it
  advances when every task is blocked; wakers count the woken task as
  running first). `main.go`: `RunMain` (root scope, SIGINT/SIGTERM, exit
  status 0/1/70, crash bundles).
- `caps.go` real capabilities (fs rooting per core 6.3), `http.go`,
  `json.go`, `log.go`, `testing.go` (test runner, fakes, `run_main`),
  `std_gen.go` (generated).

## Known gaps (2026-10-08)

- Generic code (2026-10-08): without `--mono`, `==`/`<` on a type
  parameter box both operands (`rt.Eq`/`rt.Cmp`) and constraint method
  calls convert to an interface; generic workloads run 1.6x to 17.8x
  slower than Go generics, 0.49x to 2.26x with `--mono`. Checked Int
  arithmetic still keeps small methods over Go's inlining budget
  (`gotchas/checked-arith-inline-budget.md`). A fallible constraint
  method on a type whose method cannot fail faults
  (`gotchas/generic-constraint-no-adapter.md`). Polymorphic recursion is
  rejected by Go in both modes.

- Regression tests (core 9.2): `xo test` passes `XO_REGRESSION_OUT` to the
  test binary; `std/rt/regress.go` records shrunk contract fuzz failures and
  `cmd/xo/regress.go` writes them to the module's `regressions_test.xo` after
  type checking a copy. Property test failures are not written.
- `Map.put` in update heavy loops is no longer a gap (2026-10-08): 3M
  puts over 50k keys plus 3M gets take 142 ms against Go's 45 ms (3.2x,
  was 48x); `Map.update` loops 4.4x. `List[i] = x` (`SetAt`) and
  `insert`/`remove_at` still copy the whole list (O(n) per write); the
  entry log technique would apply but needs a version check on every List
  read. Numbers: `bench/results/perf-map-2026-10-08.md`.

