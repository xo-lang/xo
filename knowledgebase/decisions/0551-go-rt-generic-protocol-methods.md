# 0551: Go backend keeps rt's generic types' protocol methods only when reached

Status: Proposed (Xo Lang 2, 2026-11-07)

## Context
After decision 0542, `xo test --backend=go` of azure's armnetwork still
needs 14.7 GB of compile memory (GOGC=25, amd64 offload host, go 1.26.0,
toolexec max RSS of the package's compile; 20.6 GB at GOGC=100) against
1.28 GB for Go's own armnetwork v8.0.0 (132k lines). Experiments on the
emitted tree (notes: ~/xo-lead/notes/xo-lang-2/gomem/notes.md), each the
same measurement:

- the emitted code is not what the compiler spends on: stubbing the 970
  monomorphized cross-module functions (20% of xo.go) saves 5.5% (13.9 GB),
  stubbing every XoJsonDec 2% and every XoJsonRead 3%; dropping every
  //line 1.4%; tests and vector constants 5.4%; replacing the 8,327
  value-marking closures with a generic rt helper saves nothing (14.5 GB).
- the compiled archive (go tool nm, text symbols) holds 267,305 functions
  and 115 MB of machine code, of which the package's own non-generic
  functions are 41,908 (24.6 MB). The rest are instantiations of rt's
  generic types and of the generic functions their methods call: Result
  46,303 symbols (11.9 MB), Option 42,122 (8.9 MB), List 31,450 (5.7 MB),
  rt.Gen 2,665 (11.9 MB), rt.JsonDec 4,031 (11.2 MB), rt.Eq 3.6 MB,
  rt.JsonRead 2.8 MB, rt.Hash 1.7 MB, rt.Cmp 1.4 MB. armnetwork's Go uses
  1,224 distinct Option types, 2,405 Result types and 723 List types.

Go compiles every method of a generic type for each instantiation in the
package that instantiates it (decision 0098;
gotchas/go-generic-method-sets-per-package.md), whether or not a method is
called, because the type's method table names them. rt's List, Set, Map,
Option, Result, the tuples (Tuple2 to Tuple64), Secret and Box carry XoEq, XoHash,
XoCmp, XoDebug, XoGenAny, XoJsonEnc, XoJsonDec, XoJsonRead and XoMark, so
each of armnetwork's 4,350 instantiations compiles all of them, and
XoGenAny alone instantiates rt.Gen[T] (a type switch over every kind) for
each element type. 0542 left these methods alone: "std's types in the
runtime package keep every method".

Removing methods from rt's generic types in the emitted tree (each
variant also without XoGenAny; toolexec max RSS and time of the module's
own package compile, GOGC=25, amd64 offload host, go 1.26.0; armstorage,
4.2 MB of Go, is the mid-size check):

| rt generic types without | armnetwork | armstorage |
|---|---|---|
| nothing (today) | 14.7 GB, 96 s | 2.28 GB, 17.7 s |
| XoGenAny | 12.6 GB (-14%) | 1.88 GB (-18%) |
| XoGenAny, XoEq, XoHash | 11.4 GB (-23%) | 1.73 GB (-24%) |
| XoGenAny, XoCmp | 12.3 GB (-16%) | 1.83 GB (-20%) |
| XoGenAny, XoDebug | 11.8 GB (-20%) | 1.79 GB (-21%) |
| XoGenAny, XoEq, XoHash, XoCmp | 10.8 GB (-27%), 71 s | 1.66 GB (-27%), 9.4 s |
| the same, with -gcflags=-l | 7.1 GB (-52%) | 1.11 GB (-51%) |

(The JSON methods cannot be left out: each user type's XoJsonEnc calls
its Option fields' XoJsonEnc directly. XoDebug is reached from every
error frame.)

## Decision
1. In std/rt, each XoGenAny, XoEq and XoHash method of a generic type
   (List, Set, Map, Option, Result, Secret, and the tuples of core.go,
   show.go and scripts/gentuples' tuples_gen.go; not Box, which the
   generator wraps around a recursive field the walk sees as the field's
   own type) is preceded by a line `//xo:proto BIT TYPE` (`//xo:proto eq
   Result`); the method runs from its doc comment to the next `}` in
   column 0. Pruning is per generic type and protocol, so a program that
   compares Options keeps no XoEq on its Results. The methods stay in
   their files. The other methods (XoDebug, the JSON methods, XoMark)
   are not marked: every type reaches Debug
   through error frames, and JSON and marking are reached from every
   user type's own methods, which 0542 keeps.
2. 0542's walk (compiler/gen/proto.xo proto_walk) records, when it reaches
   one of these generic types with a protocol bit, that bit for that
   generic type (`g.proto_rt[name]`), and keeps walking into the type
   arguments as it does today. The walk also enters a std type's fields
   (it stops at std types today, which keep every method): a std type's
   XoEq compares its List, Map and Option fields, so those generic types
   are reached when the std type is. A protocol set for every type
   (proto_all: a generated type parameter, a generic form that runs with
   an unknown argument) keeps it on every generic type too. A Set is a
   Map underneath, so reaching a Set reaches Map (and List, for
   XoGenAny).
3. The Go backend's tree writer (compiler/xo/gobackend.xo), which copies
   std/rt into the build tree, leaves out each marked method whose bit is
   not recorded for its type (unless XO_GO_PROTO=all); a marker not
   followed by a method, or a method left open at the end of the file,
   fails the copy, and internal/repotest checks that every such method
   of a generic type is marked. A program with no property test and no
   `==` on an Option, List or tuple compiles no XoGenAny, XoEq or XoHash
   for any of them.
4. Safety as in 0542: rt's generic types gain `XoProto()`, so rt.EqAny,
   rt.HashAny and rt.Gen fault with "xo internal: XoEq not generated for
   T (decision 0542; XO_GO_PROTO=all writes every one)" on a missed root
   instead of falling back to reflection. A missed root is a loud fault.
5. No run time cost: a kept method is the same code, dispatched the same
   way (an interface assertion in rt.EqAny); a dropped method is never
   called.

## Consequences
- A second stage was sized and is not proposed (2026-11-07): on the
  0551 armnetwork tree, also dropping Option's XoEq and every generic
  type's XoCmp saves 0.33 GB (7.28 and 7.14 GB to 6.95 and 6.81 GB, two
  rounds, -4.6%) and 2.5 s. A typed `==` on an Option or List of a typed
  element (rt.OptionEqBy and rt.ListEqBy, written on branch l2-0551s2,
  Go parity goldens passing) does not reach even that on armnetwork: its
  tests compare `json.Value?`, and json.Value's own XoEq compares its
  List, Option and tuple fields, so those keep XoEq (XO_GO_PROTO=why,
  which now names the root that first reaches each runtime generic
  type). XoCmp would need roots of its own (`<` and the other orderings,
  sort, min and max, Ord constrained std arguments, derive Ord).
- What the 0551 tree's compile still holds (go tool nm, text symbols):
  189,265 functions and 70.9 MB against 267,305 and 115 MB before 0542
  and 0551; the package's own functions 17.6 MB, other generic copies
  12.6 MB, rt.JsonDec[T] 9.7 MB (3,521 copies; the commonest cases kept
  generic and the rest made non-generic in 0e5c67778: armnetwork on main
  9.10 to 8.12 GB, on the 0551 tree 7.15 to 6.35 GB), closures 2.8 MB,
  Result's XoDebug 2.6 MB, rt.JsonRead[T] 2.5 MB (its five cases are the
  fast decode path; making it non-generic cost 3% of decode speed, so it
  stays).
- std/rt's hand written Go compares a generic value with EqAny in one
  place (http.go's request query, a Map[string, string]); it now compares
  the two maps directly (queryEq), since the first armnetwork run with
  this change faulted there ("XoEq not generated for rt.Map[string,string]",
  7 of 9 tests): the 0542 safety net works for the runtime too.
- The implementation (branch l2-0551, cba828042) on armnetwork, `xo test
  --backend=go`'s tree built on the amd64 offload host with xo's go build
  flags (-trimpath -mod=mod -gcflags=all=-dwarf=false), GOGC=25, one run
  each: the package's compile 7.18 GB and 54.3 s against 9.14 GB and
  67.8 s for main's xo (6c13fd76c), -21% and -20%; the 9 tests pass with
  both. (Main's tree is 23.7 MB of Go; the 14.7 GB of the table above is
  an older 41.5 MB emission, so the table's percentages, not its
  absolute figures, carry over.) The tree keeps XoEq on List, Option and
  the tuples and no XoHash or XoGenAny on any generic type.
- armstorage, the same way: 1.38 GB and 9.0 s against 1.68 GB and
  10.9 s, -18% and -17%; its 5 tests pass with both. (The table's
  experiment rows kept DWARF on, so their absolute figures are higher.)
- rt's Go varies by program (which generic types keep which methods);
  GOCACHE keeps one rt archive per variant, and a program's variant is
  stable, so its incremental builds stay cached.
- std/rt's hand written Go may call a marked method directly only from a
  method with the same marker (Set.XoHash calls its Map's XoHash);
  elsewhere it goes through rt.Eq or rt.Hash, whose type parameters the
  walk roots (std/go_proto_free.txt's review covers them). Today no
  other direct call exists (Buf.XoDebug calls List.XoDebug, which is not
  marked); go build of the tree fails on a new one.
- Only the Go backend changes; native, LLVM and wasm do not use rt.

## Alternatives
- Per-instantiation pruning: the methods become generic free functions
  the generator calls directly where the static type is known, with a
  registry keyed by dynamic type for the `any` paths (rt.EqAny on an
  interface value, interface typed map keys). It compiles only the
  instantiations used, but each dynamic dispatch becomes a map lookup
  (amd64 host, go 1.26: rt.EqAny's interface assertion and call 2.0 ns;
  a map keyed by the interface's type word 5.1 ns, sync.Map 8.0 ns, a
  map keyed by reflect.Type 9.1 ns) and each used instantiation needs an
  init time registration. The next step if per-group pruning leaves too
  much; a larger change in gen and rt.
- A monomorphization ADR (shared cross-module generics): measured at 5.5%
  of the peak, not the cost.
- `-gcflags=-l` on the big package: 9.6 GB (-35%), but it turns off
  inlining for the code that runs; a stopgap for `xo test --backend=go`
  only, separate from this decision.
- Splitting the package into chunks: the peak is the instantiations,
  which each chunk would repeat (0098).
