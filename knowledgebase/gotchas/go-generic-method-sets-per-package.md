# Every Go package recompiled the runtime's generic method sets

Status: Observed (2026-10-09, Go 1.26.1, darwin/arm64). Fixed the same day.

Symptom: a 6 line hello world added about 34 MB to the Go build cache and
took 280 MB of compiler memory. Its own package and package main were each
a 17 MB archive with about 41,000 symbols, almost all instantiations of
`rt.Option`, `rt.Result`, `rt.List`, `rt.Map`, `rt.Set`, `rt.Tuple2`, and
the generic functions their methods call (`rt.Gen`, `rt.JsonDec`, `rt.Eq`,
...) over the runtime's own types (`FileInfo`, `Http_Response`,
`memListener`). Parallel test suites ran out of memory.

Cause: Go's compiler (unified IR, `cmd/compile/internal/noder/reader.go`)
compiles, in every package that reads an instantiated generic type, every
method of that type plus its wrappers, dictionary, and type descriptor:
`needWrapper` adds instantiated types to `needWrapperTypes` even when they
come from an imported package (the TODO in `importedDef`: it cannot tell
that the importer already instantiated them), and the shaped method
bodies go to `todoBodies`. Reading a type reads its field types and the
signatures of all its methods, so mentioning `*rt.Task` or `rt.Os` reads
the whole runtime API: about 80 instantiations (each `Result[T, E]`
brings `Option[T]` and `Option[E]` through `ok()`/`err()`, each
`List[T]` a `Set[T]`, `Map[T, Unit]`, `Tuple2`, and the hash table types
through `to_set()`), times 10 to 50 methods each. Package level generic
functions, unlike methods, are compiled only where they are called.

Fix:
- The runtime's generic types keep only the protocol methods that
  runtime code reaches through interfaces (`XoEq`, `XoHash`, `XoCmp`,
  `XoDebug`, `XoJsonEnc`, `XoJsonDec`, `XoJsonRead`, `XoGenAny`, `XoMark`,
  `M_display`, `M_debug`, the fx codec's `fxSlice`/`fxFrom`/`fxPairs`/
  `fxFrom2`). Prelude methods and the helpers the generator calls are
  functions: `rt.List_len[T](tk, xs)`, `rt.Map_put(tk, &m, k, v)`,
  `rt.List_At(xs, i, loc)`. `methodIsGoMethod` routes the std methods of
  List, Map, Set, Option, Result, Chan, Secret, and json.Patch to that
  form (`rtFuncMethods` in internal/gen/decl.go). `TestGenericMethodSets`
  (std/rt/methodset_test.go) fails when a generic runtime type gains
  another method.
- Package main names no runtime type: the closure that runs `F_main` is
  `XoMain` in the main module's package (`xo_main.go`, `//go:noinline`,
  since inlining it into main brings the types back), and test mains pass
  each table as an `any` (`XoTests`, `rt.RunTestTables`).
- Side effect: std containers no longer satisfy Xo interfaces through Go
  methods (`size(xs)` with `interface Sized { fn len(self) -> Int }`).
  Specialized code wraps them in a generated adapter like any value whose
  methods differ, and the `--no-mono` adapter registry now records std
  container instantiations too (golden `std_iface_methods`).

Result: hello world's package 17.6 MB to 2.9 MB (7,700 symbols), package
main 17.6 MB to 13 KB, compile of the package 286 MB to 123 MB RSS; see
bench/results/binary-size-2026-10-09.md.

Avoid: do not add a method to a generic runtime type unless runtime code
calls it through an interface; add a function `<Type>_<name>` instead.
Do not let generated package main mention a runtime type, and keep
`XoMain` and `rt.RunTestTables` not inlined. To find what a package
instantiates, `go tool nm -size <archive>` and group symbols by the name
before the type arguments.
