# 0150: Order of the remaining self hosting stages, the switch-over, and the SSA builder in stages through `xo ssadump`

Status: Accepted (2026-10-10, by the owner's go-ahead on the lead's
proposal; the owner may revise the order). SSA stage 1 (the core
subset) implemented in `selfhost/ssa/` (Xo: ir.xo, build.xo, expr.xo,
stmt.xo, call.xo, show.xo, main.xo), `internal/ssa/stages.go`
(`BuildStages`, `BuildRaw`, `SplitModule`, the `see` hook of
build.go), `cmd/xo/ssadump.go` (hidden `xo ssadump`, test
ssadump_test.go), the checker's native mode and records
(`selfhost/check/info.xo`, load.xo `load_native`), and
`internal/selfhost/ssa_test.go` (`TestSelfhostSSA`,
`TestSelfhostSSAMutations`). SSA stage 2 (interfaces and dynamic
values, method values, `derive Ord` order, the generated equality and
order helpers) implemented 2026-10-11 in `selfhost/ssa/iface.xo`, with
the method value records of `selfhost/check` (`ref_subst`). SSA stage 3
(Map and Set, their higher order methods, constants of collection
types) implemented 2026-10-11 in `selfhost/ssa/map.xo`. SSA stage 4
(`requires`, `ensures`, invariants, `defer` with its fault
continuations, loop bounds, real-time sections) implemented 2026-10-11
in `selfhost/ssa/contract.xo` and `defer.xo`. SSA stage 5 (scopes,
tasks, channels, `select`, Mutex, Atomic, std/task, os.tasks, and the
operations that wait) implemented 2026-10-11 in `selfhost/ssa/conc.xo`,
with the scopes among the open blocks of defer.xo. SSA stage 6 (the
generic intrinsics and C interop) implemented 2026-10-11 in
`selfhost/ssa/json.xo`, with `use c` bound in the Xo checker
(`selfhost/check/cimport.xo`, decision 0152). SSA stage 7 (test
binaries) implemented 2026-10-11 in `selfhost/ssa/testmode.xo`, with
`xo ssadump --test`. SSA stage 8 (the passes: pruneUnwind,
addLandings, cleanup, InsertRC with sinkLandings and pruneLandings, and
the verifier) implemented 2026-10-11 in `selfhost/ssa/passes.xo`,
`rc.xo`, and `verify.xo`, with the stages `verify` and `mutants` of
`xo ssadump`. Evidence:
`bench/results/selfhost-ssa-2026-10-10.md`. Fourth step of decision
0130, after the lexer (0076), the parser (0127), and the checker (0141).

## Context

Decision 0130 left the order of the stages after the checker and the
point where the Xo compiler replaces the Go one to their own decision.
The lexer, parser, and checker are self hosted and identical to Go on
every repository run. What remains is the code generation half: the SSA
lowering (internal/ssa, about 14,800 lines, shared by the two native
backends), the arm64 emitter (internal/arm64), the LLVM emitter
(internal/llvm's SSA path), and the Go code generator (internal/gen).

The SSA builder is the largest piece left and the one every native
emitter depends on. Like the checker it does many things at once
(types, functions and generics, SSA construction, expressions,
patterns, calls, closures, interfaces, maps, contracts, deferred code,
concurrency, the std implementations, test binaries, and the passes
after building), so it needs a dump both implementations print and a
plan to compare it a part at a time.

## Decision

**Order of the remaining stages** (proposed by the lead, the owner did
not object):

1. **SSA lowering** (this decision), shared by both native backends.
2. **arm64 emitter**: with it, the compiler can be one native binary
   with no Go toolchain and no clang (the arm64 backend assembles and
   links itself on darwin/arm64 and linux/arm64, decision 0049).
   Note (2026-10-11, decision 0157): this claim was wrong when written.
   The arm64 backend does not assemble or link itself: decision 0049
   has the system C compiler driver (`cc`, or `$XO_CC`) assemble the
   generated `.s` and run the linker, and the first native build
   compiled the C runtime (std/llrt) with that compiler (clang, or gcc
   on Linux). Decision 0157 ships the runtime prebuilt in release
   archives, which removes the runtime compile; the assembler and
   linker remain, so the self hosted arm64 compiler needs `cc` (on
   macOS the Command Line Tools) until the arm64 backend writes object
   files and links them itself. It needs no Go toolchain for programs
   that do not use time zones by name (those read Go's zoneinfo.zip,
   `XO_ZONEINFO`).
3. **WebAssembly backend** (added by the owner 2026-10-11): a new
   emitter from the SSA to `.wasm`, written in Xo after the arm64
   switch-over, so the playground can run the Xo compiler itself as wasm
   with no Go compiler or linker in the browser (decision 0144). First
   experiment: the core subset lowered to WAT, assembled with `wat2wasm`,
   run under wazero, output diffed against the Go backend's goldens.
4. **LLVM emitter**: the other native backend, for the targets arm64
   does not cover.
5. **Go code generator** last: it is needed only for the Go backend,
   which already needs the Go toolchain.

**Order amended (owner, 2026-10-11).** Self hosting moves ahead of other
work, and the stages that end the double maintenance (every language
change made in the Go implementation and in its Xo mirror) come first:
(1) arm64 emitter and the switch-over, (2) LLVM emitter (2,737 lines of
Go, consumes the same SSA), (3) Go code generator (11,354 lines, from the
checked tree; compared as a text diff of the generated Go), (4) retire the
Go implementation once the full suite passes on the self hosted compiler,
(5) WebAssembly backend, written only in Xo (no Go twin, so no double
maintenance whenever it lands; it moves the compiler in the browser
later), (6) the runtime pilot (std/llrt's xo_unicode.c in Xo) and the
runtime port. Items 3 to 5 of the list above keep their content; only
their order changes.

**Switch-over point**: the Go implementation stays the reference until
the Xo compiler (lexer, parser, checker, SSA builder and passes, arm64
emitter) compiles itself to a fixed point on arm64 hosts (the binary it
builds of itself builds a byte identical binary) and passes the arm64
goldens. From then the released `xo` for arm64 hosts is the self hosted
binary for native builds (`--backend=arm64`, `--no-gc`); the Go
implementation is kept, mirrored by the differential tests, for the Go
and LLVM backends until their stages are ported, and is removed from
the build when the Go code generator is ported and the full suite passes
on the self hosted compiler. The owner may move the point (for example
to after the LLVM emitter).

**The dump.** `xo ssadump [--stage=S] [--each] [--bench=N] <dirs...>`
(hidden) checks dirs as a native build does (check.Options.Native: the
implementation modules of std/native are loaded and checked) and prints
the SSA module after each stage named, in the form of
`ssa.Module.String`:

```
== stage <build|unwind|landings|cleanup|rc>
type <Name> = struct { ... } | enum { ... }
func <name>(<params>) -> <results> {
b0(...):
  v3: Int = add v1, v2 @ <file>:<line>:<col>
  ...
}
== errors
<pos>: the SSA builder does not support ...
```

- `build` is the builder's output before any pass (blocks and values
  in creation order, unreachable blocks and trivial block parameters
  still there, value numbers as the builder gave them); it is printed
  even when the builder reports errors. `unwind` is after pruneUnwind,
  `landings` after addLandings, `cleanup` after cleanup (what
  `ssa.Build` returns), `rc` after InsertRC. A program that uses
  concurrency is built twice by the Go builder; the stages are those of
  the second build.
- Fault positions are relative to the working directory (LineRoot), as
  `xo build` prints them.
- `--each` builds every dir as a run of its own (`=== ssa <dir>` before
  each); a run the checker rejects prints `== rejected`. `--bench=N`
  checks once and times the builder alone (`ssa.BuildRaw`, no passes, one
  build) N times.

The self hosted builder (`selfhost/ssa`, `xo run selfhost/ssa --
[--std=DIR] [--wd=DIR] [--each] [--bench=N] <dirs...>`) prints the build
stage: the type lines and the functions it completed, then `==
skipped` and `<function>: <why>` for every function it left incomplete.

**The stages of the SSA builder**, each compared on every repository
run before the next starts:

1. **Core** (this decision): functions and methods, generic functions
   (one instance per list of type arguments), Int, Float, Float32,
   Bool, Str, Bytes, the sized integers and Rune, structs (defaults,
   `with`, nested field and element assignment), tuples, enums and
   recursive types (boxes, decision 0052), Option and Result with `?`
   and the error frames of decision 0104, `match`, `if let`, `while
   let`, patterns, `while`, `for` over ranges, Lists, `indexed()`, and
   Bytes, `break`, `continue`, `return`, calls with `var` parameters and
   receivers (places read after the arguments, decision 0142), named
   and default arguments, std calls through the native implementations
   (decision 0074) and the prelude operations lowered inline, closures
   and named functions as values, string interpolation and the Display
   and Debug text of these types (`xo.debug[T]`), slicing, `Fs` methods,
   `target`, the main wrapper of `main ! E`, the checks after calls that
   faults land on (`rt.landing`, decision 0085), and the rebuild with
   unwinding checks of a program that uses concurrency (`rt.unwinding`;
   the concurrency constructs themselves are stage 5).
2. **Interfaces and dynamic values**: `xo.iface[I]` values and calls,
   `xo.dyn` (Error, Display, Debug), method values, `derive Ord`
   comparisons, generated equality and order helpers.
3. **Collections**: Map and Set (build_map.go), their higher order
   methods, Buf, constants of collection types.
4. **Contracts and deferred code**: `requires`, `ensures`, invariants,
   `defer` with its fault continuations, loop bounds and real-time
   sections.
5. **Concurrency and I/O**: scopes, tasks, channels, `select`, Mutex,
   Atomic, `os.tasks`, waits (sleep, network, standard input).
6. **std implementations**: the generic intrinsics (JSON, SQL, any,
   cast, status), C interop (`use c`).
7. **Test binaries** (build_testmode.go).
8. **The passes**: pruneUnwind, addLandings, cleanup, InsertRC, and the
   verifier, compared at the stages `unwind`, `landings`, `cleanup`,
   and `rc` of the dump.

**Comparison rules of stage 1** (`TestSelfhostSSA`: the Xo builder built
with the go, arm64, and LLVM backends; every directory of the
repository holding .xo files, each a run of its own, from the
repository root):

- A function the Xo builder completes must be identical, text for text,
  to the Go builder's function of the same name in the build stage.
- Closures are named `<function>.fn<N>` by a counter of the whole
  build; both sides' closures are compared under the name
  `<function>.fn#<rank>`, their rank among the closures of their
  function, because constructs of later stages that create closures
  shift the counter of the Xo builder. A closure of a function the Xo
  builder skips is skipped too.
- A type the Xo builder cannot lower is a poison type (kind Bad, named
  `<unsupported: T>`): every function whose text holds it is skipped,
  and type lines holding it are not printed. Every other type line of
  the Xo builder must be one of the Go builder's.
- A run the Go checker rejects must be rejected by the Xo checker; a
  run without `main` is built by neither. Runs with `use go` or `use c`
  are not compared (the Xo checker does not bind them), nor runs whose
  only errors are holes (XO0900, which the Xo checker does not report).
- Functions skipped, and functions the Xo builder never declares
  (callees of skipped code: map helpers, JSON encoders), are counted and
  reported, not compared.

`TestSelfhostSSAMutations` mutates the single directory programs with
`main` the Go checker accepts (an operator swapped for another of its
kind, an integer or Bool literal changed, a statement line dropped or
doubled, a `?` removed) and applies the same rules to every mutation;
one either checker rejects must be rejected by both.

**Comparison rules of stage 2** (2026-10-11, interfaces and dynamic
values: selfhost/ssa iface.xo), in addition to those of stage 1:

- An interface value carries the number of its concrete type (the
  `xo.tid` field, the `const` in its `xo.ifeq` function), given in build
  order at the first conversion of that type. A construct the Xo
  builder skips can hide conversions (and queued bodies) the Go builder
  made, so a number the Xo builder gives after it first skipped
  anything is not compared: the functions that use it are skipped
  ("an interface value's type number given after skipped code").
- Generated functions whose bodies declare functions (the method, show,
  and equality functions of interface values, `xo.eqc`, `xo.cmp`, and
  from stage 3 the map helpers and constants) are built when asked for,
  as the Go builder's genFunc builds them, so the queue of bodies (and
  with it the type numbers) is in the Go builder's order. Debug text,
  slices, list concatenation, frame arguments, and zero functions stay
  built after the current body (their bodies declare nothing).
- A method value is the closure literal check.Info.MethodValueLit makes
  (`fn(p0, ...) { xo.recv.m(p0, ...) }`); the Xo builder makes the same
  literal with its checker records at offsets past every source file
  (1,000,000 + 16 per method value, the lines and columns of the
  selector), so the call in it lowers as any call. The Xo checker
  records the substitution of a method value (`ref_subst`, the Go
  checker's `rec.subst`).
- A function holds a poison type when a value, result, or capture has
  a type named `<unsupported: ...>` or a call or closure names one (the
  function's text was searched before, which also matched a string
  constant holding that text in selfhost/ssa's own dump code).

**Comparison rules of stage 3** (2026-10-11, collections: selfhost/ssa
map.xo): none new. The map helpers are generated when asked for (rule
of stage 2); a packed closure (`Map.update`) is compared as any
closure.

**Comparison rules of stage 4** (2026-10-11, contracts and deferred
code: selfhost/ssa contract.xo, defer.xo): none new. The landings the
Go builder places while it builds (flushLanding: an operation that may
fault in a block with deferred code, and inside deferred code with its
fault continuations) are part of the build stage and compared with it;
the landings of the pass addLandings are stage 8. The Xo checker
records what the builder reads: the `result` binding of an ensures
clause (`result_vars`), the field bindings of an invariant
(`inv_vars`), and the constant of a loop bound (`const_int`, now
public). The scopes of stage 5 are not among the open blocks: a
function with one is skipped.

**Comparison rules of stage 5** (2026-10-11, concurrency and I/O:
selfhost/ssa conc.xo): none new. Only the second build of a program
that uses concurrency (the one with unwinding checks) is printed and
compared, as before. A scope is an open block of defer.xo (a frame
whose `sc` names it, build_defer.go exitEntry.scope), so `return`,
`?`, `break`, and `continue` close it (mode 1), unwinding runs the
deferred code inside the innermost scope and jumps to its unwind block,
and an operation that may fault inside it gets a landing (the port's
`any_defer` now means "a `defer` or a scope", gotchas/selfhost-ssa-
port-shortcut-flags.md). Spawn sites are numbered by the checker key of
their call in check.Inspect order (conc.xo walk_*), as the Go builder
numbers them by AST node.

**Comparison rules of stage 6** (2026-10-11, std implementations:
selfhost/ssa json.xo; selfhost/check cimport.xo, decision 0152):

- The generated functions of the intrinsics (`xo.json.enc[T]`,
  `xo.json.dec[T]`, `xo.sql.dec[T]`) are built when asked for, as the
  Go builder's genFunc builds them (they declare functions: the
  encoders and decoders of their fields, map helpers, Row accessors),
  and compared as any function.
- Runs with `use c` are compared: the Xo checker binds a header from
  the declaration file the Go binder cached (decision 0152), so the Go
  checker must have bound it first (TestSelfhostSSA computes the Go
  builds before it runs the Xo builder); a run whose binding is not in
  the cache is reported as `== unsupported use c ...` and counted, not
  compared. Runs with `use go` stay unsupported (they never build
  natively).
- What the Go builder itself does not support (it reports an error and
  the build fails: a JSON decoder of a sized integer, a conversion
  between instances that differ only in effects, `dbg`, holes) the Xo
  builder skips, and such functions are counted as skipped.

**Comparison rules of stage 7** (2026-10-11, test binaries: selfhost/ssa
testmode.xo): the test binary of every repository run whose .xo files
declare a test block (a line starting with `test "`) is built by both
builders (`xo ssadump --test`, `selfhost/ssa --test`: ssa.Options.Tests,
without --real) and compared by the rules of stage 1; so are the test
binaries of the mutations with test blocks. The entry `main` and the
property test generators `xo.gen[T]` are built when asked for, as the
Go builder's genFunc builds them. The Xo checker records the `os`
binding and the `for` parameters of each test block (Info.TestOs,
Info.TestParams), keyed by the block's file and offset.

**Comparison rules of stage 8** (2026-10-11, the passes: selfhost/ssa
passes.xo, rc.xo, verify.xo):

- `selfhost/ssa --stages` prints, after the build stage, the module after
  each pass (`unwind`, `landings`, `cleanup`, `rc`), then `verify` (the
  verifier's verdict after rc: `ok`, or its errors sorted) and `mutants`
  (the verdicts on the module with one retain or release removed: the
  first `MutantsPerFunc` = 6 of every function, one line per mutant,
  `<function> b<block> <index>: ` and its errors sorted). `xo ssadump
  --stage=verify,mutants` prints the Go verifier's (ssa.VerifyMutants);
  neither is part of `--stage=all`.
- The stages after the build are compared only on runs both builders
  build completely: the Go builder prints them only for a build without
  errors, the Xo builder only when it skipped no function. Every
  function of each stage must be identical (closures by rank, as in the
  build stage), the type lines are compared as sets, and the lines of
  `verify` and `mutants` as sorted lists (the two builders make
  functions in different orders).
- The checks the builder makes are recorded in creation order
  (b.checks: function, check call, callee, operation) for pruneUnwind;
  the blocks split off after a check are recorded (landRest) for
  mergeJumps. A pass works on a function moved out of the module;
  cleanup and InsertRC renumber values in the Go builder's order (block
  order, parameters first) by rebuilding the value table, and removed
  blocks are dropped from the block list with every block index
  renumbered.
- The test binaries of stage 7 are compared at every stage too.

**The Xo builder** keeps the IR in tables: a module's types are a List
interned by name, a function's values (indexed by their ID), blocks,
and edges are Lists, and every reference is an index (-1 for none). The
function under construction is a value moved out of the module and back
(`begin`, `end`); the module keeps its signature meanwhile, for
recursive calls. SSA construction is Braun et al.'s algorithm as in the
Go builder, with variables keyed by checker local and block. Places
(`x.f[i] = v`) are a list of steps with the Go builder's per level
caches. The checker's records are read through selfhost/check info.xo,
which keys every node by its file, span, and kind.

## Consequences

- A change to internal/ssa's builder in the core subset must be made in
  selfhost/ssa in the same change, or `go test -short ./...` fails
  (the short run compares every 25th repository run on the go
  backend).
- The Xo checker has a native mode (`--native`, `check.Options.Native`
  ported: std/native modules for the prelude and every std module
  loaded, native/rt in signature mode), identical to the Go checker's
  typedump in native mode on 608 of 614 runs (the 6 others differ
  outside native mode too: holes, XO0900, and one habit run).
- The Xo checker records what the builder reads: natural types,
  constructors (keyed by the expression Go keys: the selector for
  `Kind.Newline`, not its receiver), type calls, the substitution of a
  call, pattern types, variants, and newtypes, the parameter bindings of
  bodies and closures, and the declaration file of every function
  (methods had none).
- A by value receiver is read before a call's arguments: `b.f(b.g())`
  where g changes b sees the old b (gotchas/receiver-read-before-args.md);
  the port binds such arguments first.

## Alternatives

- **Port the arm64 emitter first** (it is smaller than the SSA builder):
  it consumes the SSA after the passes, so it could only be compared on
  modules the Go builder makes, and a self hosted compiler would still
  need the Go builder.
- **LLVM before arm64**: the LLVM path needs clang to assemble and link,
  so it cannot give a compiler with no external tools. (Note
  2026-10-11, decision 0157: neither can the arm64 path today, which
  needs `cc` to assemble and link; it needs a C compiler driver, not
  clang specifically, and no compile of the runtime with a release's
  prebuilt runtime.)
- **Compare after cleanup only**: renumbering hides the order values are
  made in, which is where most porting mistakes show; the build stage
  is stricter, and the passes get their own stage.
- **Compare closures by the global counter**: every construct of a later
  stage that makes a closure would make every later closure of the
  program differ.
